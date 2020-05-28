package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/olekukonko/tablewriter"
)

func init() {
	flag.StringVar(&discordToken, "dt", "", "Discord Bot Token")
	flag.StringVar(&riotToken, "rt", "", "RIOT API Token")
	flag.StringVar(&riotRegion, "r", "", "RIOT API Region")
	flag.Parse()

	if false == isFlagPassed("dt") {
		discordToken = os.Getenv("LOLBOT_DISCORD_TOKEN")
	}

	if false == isFlagPassed("rt") {
		riotToken = os.Getenv("LOLBOT_RIOT_TOKEN")
	}

	if false == isFlagPassed("r") {
		riotRegion = os.Getenv("LOLBOT_RIOT_REGION")
	}
}

var discordToken string
var riotToken string
var riotRegion string
var riotClient RiotClient

func main() {
	if len(discordToken) == 0 {
		fmt.Printf("Discord bot token not provided. Provide a token using the '-dt' argument or the LOLBOT_DISCORD_TOKEN environment variable.")
		return
	}

	if len(riotToken) == 0 {
		fmt.Printf("RIOT API token not provided. Provide a token using the '-rt' argument or the LOLBOT_RIOT_TOKEN environment variable.")
		return
	}

	if len(riotRegion) == 0 {
		riotRegion = "EUN1"
		log.Print("RIOT Region not provided. Defaulting to: " + riotRegion)
	}

	riotClient = NewRiotClient(riotRegion, riotToken, 10)

	dg, err := discordgo.New("Bot " + discordToken)

	if nil != err {
		fmt.Println("Error creating Discord session: ", err)
	}

	// Run events in their own go routines.
	dg.SyncEvents = false

	// Register messageCreate as a callback for the messageCreate events.
	dg.AddHandler(messageCreate)

	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
		dg.Close()
		return
	}

	fmt.Println("LoL Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// check if the message is "!lolbot"
	if strings.HasPrefix(m.Content, "!lolbot") {
		args := strings.Split(m.Content, " ")

		log.Printf("[INFO][Recei][%s][%s]%v", m.ChannelID, m.Author.ID, args)

		sendMessage, err := s.ChannelMessageSend(m.ChannelID, "Working on it ...")

		msg := ""

		if len(args) == 1 {
			msg = fmt.Sprintf("Hi %s :wave:\nI will show the ranks of everyone in the current game of the summoner you specify. Try it out like this:\n!lolbot SomeSummoner", m.Author.Username)
		}

		if len(args) >= 2 {
			summonerName := strings.Join(args[1:], " ")
			match, err := riotClient.GetLiveMatchBySummonerName(&summonerName)

			if err == nil {
				buf := new(bytes.Buffer)
				table := tablewriter.NewWriter(buf)
				table.SetAlignment(tablewriter.ALIGN_LEFT)
				table.SetCaption(true, fmt.Sprintf("Current match: %s", summonerName))
				table.SetHeader([]string{"Team", "Champion", "Summoner", "Solo", "Flex"})

				var bt, rt []*discordgo.MessageEmbedField

				headers := []string{"Champion", "Summoner", "Solo", "Flex"}
				for i := range headers {
					bt = append(bt, &discordgo.MessageEmbedField{
						Name:   headers[i],
						Value:  "\u200b",
						Inline: true,
					})
					rt = append(rt, &discordgo.MessageEmbedField{
						Name:   headers[i],
						Value:  "\u200b",
						Inline: true,
					})
				}

				for i := range match {
					c := []*discordgo.MessageEmbedField{
						{
							Value:  match[i].Champion,
							Inline: true,
						},
						{
							Value:  match[i].SummonerName,
							Inline: true,
						},
						{
							Value:  match[i].Solo,
							Inline: true,
						},
						{
							Value:  match[i].Flex,
							Inline: true,
						},
					}

					if match[i].Team == "BLUE" {
						bt = append(bt, c...)
					}
					if match[i].Team == "RED" {
						rt = append(rt, c...)
					}
				}

				me := &discordgo.MessageEmbed{
					Author: &discordgo.MessageEmbedAuthor{},
					Color:  0x0000FF, // Blue
					Title:  "Blue Team",
					Fields: bt,
					Image: &discordgo.MessageEmbedImage{
						URL: "https://cdn.discordapp.com/avatars/119249192806776836/cc32c5c3ee602e1fe252f9f595f9010e.jpg?size=2048",
					},
					Thumbnail: &discordgo.MessageEmbedThumbnail{
						URL: "https://cdn.discordapp.com/avatars/119249192806776836/cc32c5c3ee602e1fe252f9f595f9010e.jpg?size=2048",
					},
					Timestamp: time.Now().Format(time.RFC3339), // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
				}

				s.ChannelMessageSendEmbed(sendMessage.ChannelID, me)

				log.Println(err)

				d := [][]string{}
				for i := range match {
					d = append(d, []string{
						match[i].Team,
						match[i].Champion,
						match[i].SummonerName,
						match[i].Solo,
						match[i].Flex},
					)
				}
				table.AppendBulk(d)
				table.Render()
				msg = "```" + buf.String() + "```"
			}
		}

		if "" == msg {
			msg = "Hmh, it looks like something went wrong. Check if the summoner name is spelled correctly and that the summoner is currently in a match."
		}

		_, err = s.ChannelMessageEdit(sendMessage.ChannelID, sendMessage.ID, msg)

		if err != nil {
			log.Printf("[ERRO][Reply][%s][%s]%v\n%s", m.ChannelID, m.Author.ID, args, err)
		}
	}
}
