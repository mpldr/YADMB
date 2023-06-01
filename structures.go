package main

import (
	"github.com/TheTipo01/YADMB/Queue"
	"github.com/bwmarrin/discordgo"
	"sync"
	"sync/atomic"
)

// Server holds all info about a single server
type Server struct {
	// The queue
	queue Queue.Queue
	// Voice connection
	vc *discordgo.VoiceConnection
	// Custom commands, maps a command to a song
	custom map[string]*CustomCommand
	// Frames
	frames int
	// Quit channel
	skip chan struct{}
	// Whether the job scheduler has started
	started atomic.Bool
	// Whether to clear the queue
	clear atomic.Bool
	// Guild ID
	guildID string
	// Voice channel where the bot is connected
	voiceChannel string
	// Number of people in the voice channels of the guild
	voiceChannelMembers map[string]*atomic.Int32
	// Whether the bot is paused
	paused atomic.Bool
	// Channel for pausing
	pause chan struct{}
	// Channel for resuming
	resume chan struct{}
	// Wait group for waiting for spotify to finish before lowering the clear flag
	wg *sync.WaitGroup
}

// CustomCommand holds data about a custom command
type CustomCommand struct {
	link string
	loop bool
}

// YtDLP structure for holding yt-dlp data
type YtDLP struct {
	Duration         float64          `json:"duration"`
	Thumbnail        string           `json:"thumbnail"`
	Extractor        string           `json:"extractor"`
	ID               string           `json:"id"`
	WebpageURL       string           `json:"webpage_url"`
	Title            string           `json:"title"`
	RequestedFormats RequestedFormats `json:"requested_formats"`
}

// RequestedFormats is used to detect if an audio only codec is available
type RequestedFormats []struct {
	Resolution string `json:"resolution"`
}

// SponsorBlock holds data for segments of sponsors in YouTube video
type SponsorBlock []struct {
	Category string    `json:"category"`
	Segment  []float64 `json:"segment"`
	UUID     string    `json:"UUID"`
}

// Config holds data parsed from the config.yml
type Config struct {
	Token        string `fig:"token" validate:"required"`
	Owner        string `fig:"owner" validate:"required"`
	ClientID     string `fig:"clientid" validate:"required"`
	ClientSecret string `fig:"clientsecret" validate:"required"`
	DSN          string `fig:"datasourcename" validate:"required"`
	Driver       string `fig:"drivername" validate:"required"`
	LogLevel     string `fig:"loglevel" validate:"required"`
}
