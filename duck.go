package main

import (
	"errors"
	"strconv"
	"time"

	"flag"
	"github.com/sorcix/irc"
	"log"
)

type Settings struct {
	Host     string
	Name     string
	AwayText string
	Channels []string
}

func main() {
	var host = flag.String("host", "localhost:6667", "IRC server")
	var nick = flag.String("nick", "duck", "IRC nickname")
	var away = flag.String("away", "Hi! Tell me about your problems :⊃", "Away text")
	flag.Parse()

	Run(Settings{
		Host:     *host,
		Name:     *nick,
		AwayText: *away,
		Channels: flag.Args(),
	})
}

// Run opens connections via RunOnce and retries when they fail, backing off exponentially.
func Run(settings Settings) {
	baseDelay := 30 * time.Second
	nextDelay := baseDelay
	delayFactor := 5
	maxDelay := 6 * time.Hour

	// Endless loop: if a connection fails, reestablish it after some time.
	for {
		errChan := make(chan error)
		successChan := make(chan bool)

		// Start connection
		log.Println("connecting to", settings.Host)
		go func() {
			errChan <- RunOnce(settings, successChan)
		}()

		// See if it succeeds
		var err error
		select {
		case <-successChan:
			// Connected, see if it stays up
			log.Println("connected", settings.Host)
			select {
			case <-time.After(3600 * time.Second):
				// Stayed up => reset back-off interval
				log.Println("connection is stable", settings.Host)
				nextDelay = baseDelay
				err = <-errChan
			case <-errChan:
				err = <-errChan
			}
		case <-errChan:
			err = <-errChan
		}

		if err != nil {
			log.Println("error:", err)
		}

		// Wait until it's finished
		close(errChan)
		close(successChan)

		// Wait before reconnect
		log.Println("sleeping for:", nextDelay)
		time.Sleep(nextDelay)
		nextDelay *= time.Duration(delayFactor)
		if nextDelay > maxDelay {
			nextDelay = maxDelay
		}
	}
}

// RunOnce opens one connection and maintains it until it dies.
// It'll send true to the channel once the connection has been established.
func RunOnce(settings Settings, success chan bool) (err error) {

	// Open connection
	conn, err := irc.Dial(settings.Host)
	if err != nil {
		return
	}

	// Register and await welcome
	err = conn.Encode(&irc.Message{
		Command: irc.NICK,
		Params:  []string{settings.Name},
	})
	if err != nil {
		return
	}
	err = conn.Encode(&irc.Message{
		Command:  irc.USER,
		Params:   []string{settings.Name, "0", "*"},
		Trailing: settings.Name,
	})
	if err != nil {
		return
	}
	message, err := conn.Decode()
	if err != nil {
		return
	}
	if message.Command != irc.RPL_WELCOME {
		err = errors.New("expected welcome, got" + strconv.Quote(message.Command))
		return
	}

	// Set status
	if settings.AwayText != "" {
		err = conn.Encode(&irc.Message{
			Command:  irc.AWAY,
			Trailing: settings.AwayText,
		})
		if err != nil {
			return
		}
	}

	// Join channels
	for _, channel := range settings.Channels {
		err = conn.Encode(&irc.Message{
			Command: irc.JOIN,
			Params:  []string{channel},
		})
		if err != nil {
			return
		}
		time.Sleep(2)
	}

	// React to commands
	for {
		message, err := conn.Decode()
		if err != nil {
			return err
		}

		switch message.Command {
		case irc.PING:
			success <- true

			err = conn.Encode(&irc.Message{
				Command:  "PONG",
				Params:   message.Params,
				Trailing: message.Trailing,
			})
			if err != nil {
				return err
			}
		}
	}
}
