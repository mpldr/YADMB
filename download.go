package main

import (
	"errors"
	"github.com/TheTipo01/YADMB/queue"
	"github.com/TheTipo01/YADMB/sponsorblock"
	"github.com/TheTipo01/YADMB/youtube"
	"github.com/bwmarrin/discordgo"
	"github.com/bwmarrin/lit"
	"github.com/goccy/go-json"
	"github.com/zmb3/spotify/v2"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const youtubeBase = "https://www.youtube.com/watch?v="

// Download and plays a song from a YouTube link
func downloadAndPlay(s *discordgo.Session, guildID, link, user string, i *discordgo.Interaction, random, loop, respond, priority bool) {
	var c chan struct{}
	if respond {
		c = make(chan struct{})
		go sendEmbedInteraction(s, NewEmbed().SetTitle(s.State.User.Username).AddField(enqueuedTitle, link).SetColor(0x7289DA).MessageEmbed, i, c)
	}

	// Check if the song is the db, to speedup things
	el, err := db.CheckInDb(link)
	if err == nil {
		info, err := os.Stat(cachePath + el.ID + audioExtension)
		if err == nil && info.Size() > 0 {
			f, _ := os.Open(cachePath + el.ID + audioExtension)
			el.User = user
			el.Reader = f
			el.Closer = f
			el.TextChannel = i.ChannelID
			el.Loop = loop

			if respond {
				go deleteInteraction(s, i, c)
			}
			server[guildID].AddSong(priority, el)
			return
		}
	}

	// If we have a valid YouTube client, and the link is a YouTube link, use the YouTube api
	if yt != nil && (strings.Contains(link, "youtube.com") || strings.Contains(link, "youtu.be")) {
		err = downloadAndPlayYouTubeAPI(s, guildID, link, user, i, random, loop, respond, priority, c)
		// If we have an error, we fall back to yt-dlp
		if err == nil {
			return
		}
	}

	infoJSON, err := getInfo(link)
	if err != nil {
		if respond {
			modifyInteractionAndDelete(s, NewEmbed().SetTitle(s.State.User.Username).AddField(errorTitle, err.Error()).SetColor(0x7289DA).MessageEmbed, i, time.Second*5)
		} else {
			msg := sendEmbed(s, NewEmbed().SetTitle(s.State.User.Username).AddField(errorTitle, err.Error()).SetColor(0x7289DA).MessageEmbed, i.ChannelID)
			time.Sleep(time.Second * 5)
			_ = s.ChannelMessageDelete(msg.ChannelID, msg.ID)
		}
		return
	}

	var yt YtDLP

	// If we want to play the song in a random order, we just shuffle the slice
	if random {
		infoJSON = shuffle(infoJSON)
	}

	if respond {
		go deleteInteraction(s, i, c)
	}

	elements := make([]queue.Element, 0, len(infoJSON))

	// We parse every track as individual json, because yt-dlp
	for _, singleJSON := range infoJSON {
		_ = json.Unmarshal([]byte(singleJSON), &yt)

		el = queue.Element{
			Title:       yt.Title,
			Duration:    formatDuration(yt.Duration),
			Link:        yt.WebpageURL,
			User:        user,
			Thumbnail:   yt.Thumbnail,
			TextChannel: i.ChannelID,
			Loop:        loop,
		}

		exist := false
		switch yt.Extractor {
		case "youtube":
			el.ID = yt.ID + "-" + yt.Extractor
			// SponsorBlock is supported only on YouTube
			el.Segments = sponsorblock.GetSegments(yt.ID)

			// If the song is on YouTube, we also add it with its compact url, for faster parsing
			db.AddToDb(el, false)
			exist = true

			// YouTube shorts can have two different links: the one that redirects to a classical YouTube video
			// and one that is played on the new UI. This is a workaround to save also the link to the new UI
			if strings.Contains(link, "shorts") {
				el.Link = link
				db.AddToDb(el, exist)
			}

			el.Link = "https://youtu.be/" + yt.ID
		case "generic":
			// The generic extractor doesn't give out something unique, so we generate one from the link
			el.ID = idGen(el.Link) + "-" + yt.Extractor
		default:
			el.ID = yt.ID + "-" + yt.Extractor
		}

		// We add the song to the db, for faster parsing
		go db.AddToDb(el, exist)

		// Checks if video is already downloaded
		info, err := os.Stat(cachePath + el.ID + audioExtension)

		// If not, we download and convert it
		if err != nil || info.Size() <= 0 {
			pipe, cmd := gen(yt.WebpageURL, el.ID, checkAudioOnly(yt.RequestedFormats))
			el.Reader = pipe
			el.Downloading = true

			el.BeforePlay = func() {
				cmdsStart(cmd)
			}

			el.AfterPlay = func() {
				cmdsWait(cmd)
			}
		} else {
			f, _ := os.Open(cachePath + el.ID + audioExtension)
			el.Reader = f
			el.Closer = f
		}

		elements = append(elements, el)
	}

	server[guildID].AddSong(priority, elements...)
}

// Download and plays a song from a YouTube link, parsing the link with the YouTube API
func downloadAndPlayYouTubeAPI(s *discordgo.Session, guildID, link, user string, i *discordgo.Interaction, random, loop, respond, priority bool, c chan struct{}) error {
	var (
		result []youtube.Video
		el     queue.Element
	)

	// Check if we have a YouTube playlist, and get its parameter
	u, _ := url.Parse(link)
	q := u.Query()
	if id := q.Get("list"); id != "" {
		result = yt.GetPlaylist(id)
	} else {
		if strings.Contains(link, "youtube.com") {
			id = q.Get("v")
		} else {
			id = strings.TrimPrefix(link, "https://youtu.be/")
		}

		if video := yt.GetVideo(id); video != nil {
			result = append(result, *video)
		}
	}

	if len(result) == 0 {
		return errors.New("no video found")
	}

	if respond {
		go deleteInteraction(s, i, c)
	}

	elements := make([]queue.Element, 0, len(result))

	for _, r := range result {
		el = queue.Element{
			Title:       r.Title,
			Duration:    formatDuration(r.Duration),
			Link:        youtubeBase + r.ID,
			User:        user,
			Thumbnail:   r.Thumbnail,
			TextChannel: i.ChannelID,
			Loop:        loop,
		}

		exist := false
		el.ID = r.ID + "-youtube"
		// SponsorBlock is supported only on YouTube
		el.Segments = sponsorblock.GetSegments(r.ID)

		// If the song is on YouTube, we also add it with its compact url, for faster parsing
		db.AddToDb(el, false)
		exist = true

		// YouTube shorts can have two different links: the one that redirects to a classical YouTube video
		// and one that is played on the new UI. This is a workaround to save also the link to the new UI
		if strings.Contains(link, "shorts") {
			el.Link = link
			db.AddToDb(el, exist)
		}

		el.Link = "https://youtu.be/" + r.ID

		// We add the song to the db, for faster parsing
		go db.AddToDb(el, exist)

		// Checks if video is already downloaded
		info, err := os.Stat(cachePath + el.ID + audioExtension)

		// If not, we download and convert it
		if err != nil || info.Size() <= 0 {
			pipe, cmd := gen(el.Link, el.ID, true)
			el.Reader = pipe
			el.Downloading = true

			el.BeforePlay = func() {
				cmdsStart(cmd)
			}

			el.AfterPlay = func() {
				cmdsWait(cmd)
			}
		} else {
			f, _ := os.Open(cachePath + el.ID + audioExtension)
			el.Reader = f
			el.Closer = f
		}

		elements = append(elements, el)
	}

	if random {
		rand.Shuffle(len(elements), func(i, j int) {
			elements[i], elements[j] = elements[j], elements[i]
		})
	}

	server[guildID].AddSong(priority, elements...)
	return nil
}

// Searches a song from the query on YouTube
func searchDownloadAndPlay(query string) (string, error) {
	if yt != nil {
		result := yt.Search(query, 1)
		if len(result) > 0 {
			return youtubeBase + result[0].ID, nil
		}
	} else {
		out, err := exec.Command("yt-dlp", "--get-id", "ytsearch:\""+query+"\"").CombinedOutput()
		if err == nil {
			ids := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")

			if ids[0] != "" {
				return youtubeBase + ids[0], nil
			}
		}
	}

	return "", errors.New("no song found")
}

// Enqueues song from a spotify playlist, searching them on YouTube
func spotifyPlaylist(s *discordgo.Session, guildID, user string, i *discordgo.Interaction, random, loop, priority bool, id spotify.ID) {
	if spt != nil {
		if playlist, err := spt.GetPlaylist(id); err == nil {
			server[guildID].wg.Add(1)

			go sendAndDeleteEmbedInteraction(s, NewEmbed().SetTitle(s.State.User.Username).AddField(enqueuedTitle, "https://open.spotify.com/playlist/"+id.String()).SetColor(0x7289DA).MessageEmbed, i, time.Second*3)

			if random {
				rand.Shuffle(len(playlist.Tracks.Tracks), func(i, j int) {
					playlist.Tracks.Tracks[i], playlist.Tracks.Tracks[j] = playlist.Tracks.Tracks[j], playlist.Tracks.Tracks[i]
				})
			}

			for j := 0; j < len(playlist.Tracks.Tracks) && !server[guildID].clear.Load(); j++ {
				track := playlist.Tracks.Tracks[j]
				link, _ := searchDownloadAndPlay(track.Track.Name + " - " + track.Track.Artists[0].Name)
				downloadAndPlay(s, guildID, link, user, i, false, loop, false, priority)
			}

			server[guildID].wg.Done()
		} else {
			lit.Error("Can't get info on a spotify playlist: %s", err)
			sendAndDeleteEmbedInteraction(s, NewEmbed().SetTitle(s.State.User.Username).AddField(errorTitle, spotifyError+err.Error()).SetColor(0x7289DA).MessageEmbed, i, time.Second*5)
		}
	} else {
		sendAndDeleteEmbedInteraction(s, NewEmbed().SetTitle(s.State.User.Username).AddField(errorTitle, spotifyNotConfigure).SetColor(0x7289DA).MessageEmbed, i, time.Second*5)
	}
}

// Gets info about a spotify track and plays it, searching it on YouTube
func spotifyTrack(s *discordgo.Session, guildID, user string, i *discordgo.Interaction, loop, priority bool, id spotify.ID) {
	if spt != nil {
		if track, err := spt.GetTrack(id); err == nil {
			link, _ := searchDownloadAndPlay(track.Name + " - " + track.Artists[0].Name)
			downloadAndPlay(s, guildID, link, user, i, false, loop, true, priority)
		} else {
			lit.Error("Can't get info on a spotify track: %s", err)
			sendAndDeleteEmbedInteraction(s, NewEmbed().SetTitle(s.State.User.Username).AddField(errorTitle, spotifyError+err.Error()).SetColor(0x7289DA).MessageEmbed, i, time.Second*5)
		}
	} else {
		sendAndDeleteEmbedInteraction(s, NewEmbed().SetTitle(s.State.User.Username).AddField(errorTitle, spotifyNotConfigure).SetColor(0x7289DA).MessageEmbed, i, time.Second*5)
	}
}

// getInfo returns info about a song, with every line of the returned array as JSON of type YtDLP
func getInfo(link string) ([]string, error) {
	// Gets info about songs
	out, err := exec.Command("yt-dlp", "--ignore-errors", "-q", "--no-warnings", "-j", link).CombinedOutput()

	// Parse output as string, splitting it on every newline
	splittedOut := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")

	if err != nil {
		return nil, errors.New("Can't get info about song: " + splittedOut[len(splittedOut)-1])
	}

	// Check if yt-dlp returned something
	if strings.TrimSpace(splittedOut[0]) == "" {
		return nil, errors.New("yt-dlp returned no songs")
	}

	return splittedOut, nil
}
