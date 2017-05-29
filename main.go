package main // starting point of program

// This file heavily borrows and, in some cases, basically takes code directly
// from bwmarrin's discordgo example bots and from the airhorn bot source code
// available on github. Thanks to them, without them this wouldn't exist, and
// most of the code here is either directly taken or adapted from them.

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"io"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// struct for entries in the table of the bot's responses
type tableEntry struct {
	messageTrigger string

	responseFunction (func(*discordgo.Session, *discordgo.MessageCreate))
}

type Sound struct {
	// name of the file the sound is stored in
	Name string

	SourceFileName string

	PlayDelay int

	buffer [][]byte // buffer to store encoded PCM packets
}

// type for a set of sounds
type SoundSet struct {
	SetName string
	Sounds  []*Sound
}

// struct for a single instance of requesting a sound play
type PlayRequest struct {
	// server to play in
	GuildID string
	// server's channel to play in
	ChannelID string

	// user requesting the sound play
	UserID string

	// sound they want played
	sound *Sound
}

// sound set for trump "wrong" sounds
var TRUMPWRONG *SoundSet = &SoundSet{

	SetName: "!Wrong",

	Sounds: []*Sound{
		{"Classic", "DonaldTrumpWrongSound", 16, make([][]byte, 0)},
	},
}

var WOW *SoundSet = &SoundSet{
	SetName: "!Wow",
	Sounds: []*Sound{
		{"Classic", "Wow!", 16, make([][]byte, 0)},
	},
}

var (
	queues map[string]chan *PlayRequest = make(map[string]chan *PlayRequest)

	MAX_QUEUE_SIZE = 6

	BITRATE = 128

	discord *discordgo.Session
)

var responseTable []tableEntry = []tableEntry{
	{"!Ping", handlePingCommand},
	{"!Pong", handlePongCommand},
}

var SoundSetCollection []*SoundSet = []*SoundSet{
	TRUMPWRONG,
	WOW,
}

// note: In go, syntax is:
// "func function_name(var1Name var1Type, var2Name var2Type, ...), returnType{
// body
// }"

// Variable to hold command line parameter (authentication token)
var Token string

func init() {

	// Designates Token as target for -t Command Line Args
	flag.StringVar(&Token, "t", "", "Bot Token")

	// Parses command line args, reads them into their designated targets
	flag.Parse()
}

func main() {

	if Token == "" {
		fmt.Println("No token provided. Please run: TrashBot -t <bot token>")
		return
	}

	// load sound sets
	fmt.Println("Preloading sounds...")
	for i := 0; i < len(SoundSetCollection); i++ {
		SoundSetCollection[i].loadSet()
	}

	// Create new discord session with provided bot token
	dg, err := discordgo.New("Bot " + Token)

	discord = dg

	// Print error message and return if it didn't work
	if err != nil {
		fmt.Println("Error: Could not create discord session,", err)
		return
	}

	// register callback for ready event
	dg.AddHandler(ready)

	// register callback for message creation events
	dg.AddHandler(handleCommand)

	// register guildCreate as a callback for guildCreate events
	dg.AddHandler(guildCreate)

	// open websocket connection to discord, start listening
	err = dg.Open()

	// handle case: can't open web socket
	if err != nil {
		fmt.Println("Error opening connection", err)
		return
	}

	// Wait until termination signal or CTRL-C is recieved
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// when recieved, close session
	dg.Close()
}

func handleCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	// ignore messages with no length, or which don't start with !
	if len(m.Content) <= 0 || (m.Content[0] != '!') {
		return
	}

	// safety check - ignore messages from the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	} else {
		// iterate through commands
		for i := 0; i < len(responseTable); i++ {

			// if a matching command is found
			if responseTable[i].messageTrigger == m.Content {
				responseTable[i].responseFunction(s, m)
				return
			}
		}
		// no match found among response table. Check sound collection:

		stringParts := strings.Split(m.Content, " ")

		// 1. Check to see if the entire string matches any of the sound collections
		// also get guild and channel

		channel, _ := discord.State.Channel(m.ChannelID)
		if channel == nil {
			fmt.Println("Unable to retrieve channel")
			return
		}
		guild, _ := discord.State.Guild(channel.GuildID)
		if guild == nil {
			fmt.Println("Unable to retrieve guild")
			return
		}

		for i := 0; i < len(SoundSetCollection); i++ {
			if stringParts[0] == SoundSetCollection[i].SetName {
				if len(stringParts) > 1 { // handle case: they specified a specific sound

					var sound *Sound

					for _, s := range SoundSetCollection[i].Sounds {
						if stringParts[1] == s.Name {
							sound = s
						}
					}
					// if it didn't match any, return
					if sound == nil {
						return
					}
					queSoundPlay(m.Author, guild, SoundSetCollection[i], sound)
				}
				// Play random sound from collection
				queSoundPlay(m.Author, guild, SoundSetCollection[i], nil)
				return
			}
		}
	}
}

func handlePingCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, "Pong!")
}
func handlePongCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, "Ping!")
}

// this function should load in a sound file for a specific sound
func (s *Sound) loadSound() error {
	// open file

	path := fmt.Sprintf("SoundFiles/%v.dca", s.SourceFileName)

	file, err := os.Open(path)

	// handle errors
	if err != nil {
		fmt.Println("Error opening dca file:", err)
		return err
	}

	var opuslen int16

	for {
		// Read opus frame length from dca file
		err = binary.Read(file, binary.LittleEndian, &opuslen)

		// if thats the end of the file, return
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err := file.Close()
			if err != nil {
				return err
			}
			return nil
		}

		if err != nil {
			fmt.Println("Error reading from dca file:", err)
			return err
		}

		// read encoded pcm from dca file
		InBuf := make([]byte, opuslen)
		err = binary.Read(file, binary.LittleEndian, &InBuf)

		// should not be any end of file errors
		if err != nil {
			fmt.Println("Error reading from dca file:", err)
			return err
		}

		// append encoded pcm data to the buffer
		s.buffer = append(s.buffer, InBuf)

	}
}

// iterate through the sound set, load every sound.
func (ss *SoundSet) loadSet() {
	for i := 0; i < len(ss.Sounds); i++ {
		ss.Sounds[i].loadSound()
	}
}

// Function to call when bot recieves "ready" event from discord
func ready(s *discordgo.Session, event *discordgo.Ready) {
	// accurately describe the bot's status
	s.UpdateStatus(0, "Absolute Garbage")
}

// this function is called whenever a new guild is joined.
func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {

	// if couldn't join guild, don't try
	if event.Guild.Unavailable != false {
		return
	}

	for _, channel := range event.Guild.Channels {
		if channel.ID == event.Guild.ID {
			// intro message
			_, _ = s.ChannelMessageSend(channel.ID, "GarbageBot is ready. You asked for this.")
			return
		}
	}
}

func createPlayRequest(user *discordgo.User, guild *discordgo.Guild, sound *Sound, soundSet *SoundSet) *PlayRequest {
	channel := getCurrentVoiceChannel(user, guild)
	if channel == nil {
		fmt.Println("Warning: Failed to find channel to play in.")
		return nil
	}
	playRequest := &PlayRequest{
		GuildID:   guild.ID,
		ChannelID: channel.ID,
		UserID:    user.ID,
		sound:     sound,
	}

	// if they weren't given a sound,
	if playRequest.sound == nil {
		// grab a random sound from the set
		if (len(soundSet.Sounds) - 1) == 0 {
			playRequest.sound = soundSet.Sounds[0]
		} else {
			playRequest.sound = soundSet.Sounds[randomRange(0, len(soundSet.Sounds)-1)]
		}
	}

	return playRequest
}

func queSoundPlay(user *discordgo.User, guild *discordgo.Guild, set *SoundSet, sound *Sound) {
	play := createPlayRequest(user, guild, sound, set)
	// quit if that fails
	if play == nil {
		return
	}
	// check if you have a connection tothe guild already
	_, exists := queues[guild.ID]

	// if connection exists
	if exists {
		// if there is room in the queue, add that play to the queue
		if len(queues[guild.ID]) < MAX_QUEUE_SIZE {
			queues[guild.ID] <- play
		}
	} else { // otherwise
		queues[guild.ID] = make(chan *PlayRequest, MAX_QUEUE_SIZE)
		playSound(play, nil)
	}
}

// function to play sounds
func playSound(play *PlayRequest, vc *discordgo.VoiceConnection) (err error) {

	if vc == nil {
		vc, err = discord.ChannelVoiceJoin(play.GuildID, play.ChannelID, false, false)

		if err != nil {
			fmt.Println("Failed to play sound.")
			return err
		}
	}

	// if you need to change voice channels, do so now
	if vc.ChannelID != play.ChannelID {
		vc.ChangeChannel(play.ChannelID, false, false)
		time.Sleep(time.Millisecond * 125)
	}

	time.Sleep(time.Millisecond * 32)

	play.sound.Play(vc) // play the sound

	// if another sound is in queue, recurse and play that
	if len(queues[play.GuildID]) > 0 {
		play := <-queues[play.GuildID]
		playSound(play, vc)
		return nil
	}

	// if queue is empty, delete it
	time.Sleep(time.Millisecond * time.Duration(play.sound.PlayDelay))
	delete(queues, play.GuildID)
	vc.Disconnect()

	return nil
}

// this function blatantly ripped from airhornbot
func (s *Sound) Play(vc *discordgo.VoiceConnection) {
	vc.Speaking(true)
	defer vc.Speaking(false)

	for _, buff := range s.buffer {
		vc.OpusSend <- buff
	}
}

// this function blatantly stolen from airhornbot source code
// Attempts to find the current users voice channel inside a given guild
func getCurrentVoiceChannel(user *discordgo.User, guild *discordgo.Guild) *discordgo.Channel {
	for _, vs := range guild.VoiceStates {
		if vs.UserID == user.ID {
			channel, _ := discord.State.Channel(vs.ChannelID)
			return channel
		}
	}
	return nil
}

// this function blatantly stolen from airhornbot source code
// Returns a random integer between min and max
func randomRange(min, max int) int {
	rand.Seed(time.Now().UTC().UnixNano())
	return rand.Intn(max-min) + min
}
