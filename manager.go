package main

import (
	"fmt"
	"github.com/TheTipo01/YADMB/database"
	"github.com/TheTipo01/YADMB/queue"
	"github.com/bwmarrin/discordgo"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

// NewServer creates a new server manager
func NewServer(guildID string) *Server {
	return &Server{
		queue:               queue.NewQueue(),
		custom:              make(map[string]*database.CustomCommand),
		guildID:             guildID,
		pause:               make(chan struct{}),
		resume:              make(chan struct{}),
		skip:                make(chan struct{}),
		started:             atomic.Bool{},
		clear:               atomic.Bool{},
		paused:              atomic.Bool{},
		wg:                  &sync.WaitGroup{},
		voiceChannelMembers: make(map[string]*atomic.Int32),
	}
}

// AddSong adds a song to the queue
func (m *Server) AddSong(priority bool, el ...queue.Element) {
	if priority {
		m.queue.AddElementsPriority(el...)
	} else {
		m.queue.AddElements(el...)
	}

	if m.started.CompareAndSwap(false, true) {
		go m.play()
	}
}

func (m *Server) play() {
	msg := make(chan *discordgo.Message)

	m.paused.Store(false)

	for el := m.queue.GetFirstElement(); el != nil && !m.clear.Load(); el = m.queue.GetFirstElement() {
		// Send "Now playing" message
		go func() {
			msg <- sendEmbed(s, NewEmbed().SetTitle(s.State.User.Username).
				AddField("Now playing", fmt.Sprintf("[%s](%s) - %s added by %s", el.Title,
					el.Link, el.Duration, el.User)).
				SetColor(0x7289DA).SetThumbnail(el.Thumbnail).MessageEmbed, el.TextChannel)
		}()

		if el.BeforePlay != nil {
			el.BeforePlay()
		}

		skipped := playSound(m.guildID, el)

		// If we are still downloading the song, we need to finish writing it to disk
		if el.Downloading && (m.clear.Load() || skipped) {
			devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0755)
			_, _ = io.Copy(devnull, el.Reader)
			_ = devnull.Close()
		}

		if el.AfterPlay != nil {
			el.AfterPlay()
		}

		// Delete it after it has been played
		go func() {
			if message := <-msg; message != nil {
				_ = s.ChannelMessageDelete(message.ChannelID, message.ID)
			}
		}()

		m.queue.RemoveFirstElement()
	}

	m.started.Store(false)

	go quitVC(m.guildID)
}

// IsPlaying returns whether the bot is playing
func (m *Server) IsPlaying() bool {
	return m.started.Load() && !m.queue.IsEmpty()
}

// Clear clears the queue
func (m *Server) Clear() {
	if m.IsPlaying() {
		m.clear.Store(true)
		m.skip <- struct{}{}

		m.wg.Wait()
		m.clear.Store(false)

		q := m.queue.GetAllQueue()
		m.queue.Clear()

		for _, el := range q {
			if el.Closer != nil {
				_ = el.Closer.Close()
			}
		}
	}
}
