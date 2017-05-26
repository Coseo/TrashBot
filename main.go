package main // starting point of program

// This file heavily borrows from the discordgo api examples supplied by
// bwmarrin in the discordgo documentation.

import (
	"flag"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"os/signal"
	"syscall"
)

// struct for entries in the table of the bot's responses
type tableEntry struct {
	messageTrigger string

	responseFunction (func(*discordgo.Session, *discordgo.MessageCreate))
}

var responseTable []tableEntry = []tableEntry{
	{"!Ping", handlePingCommand},
	{"!Pong", handlePongCommand},
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
	// Tasks:
	/*
	  1. Create a new discord session with bot token.
	  2. Register the events of the bot, give it its commands.
	  3. Open a websocket connection to discord and start listening.
	  4. wait until a terminating signal is recieved.
	  5. Close discord session.
	*/

	// Create new discord session with provided bot token
	dg, err := discordgo.New("Bot " + Token)

	// Print error message and return if it didn't work
	if err != nil {
		fmt.Println("Error: Could not create discord session,", err)
		return
	}
	dg.AddHandler(handleCommand)

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
	}

}

func handlePingCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, "Pong!")
}
func handlePongCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, "Ping!")
}
