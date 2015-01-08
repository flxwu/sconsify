package ui

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fabiofalci/sconsify/sconsify"
	"github.com/jroimartin/gocui"
)

var (
	gui       *Gui
	events    *sconsify.Events
	queue     *Queue
	playlists *sconsify.Playlists
)

const (
	VIEW_PLAYLISTS = "playlists"
	VIEW_TRACKS    = "tracks"
	VIEW_QUEUE     = "queue"
	VIEW_STATUS    = "status"
)

type Gui struct {
	g              *gocui.Gui
	playlistsView  *gocui.View
	tracksView     *gocui.View
	statusView     *gocui.View
	queueView      *gocui.View
	currentMessage string
}

func InitialiseConsoleUserInterface(ev *sconsify.Events) sconsify.UserInterface {
	events = ev
	gui = &Gui{}
	queue = InitQueue()
	return gui
}

func (gui *Gui) TrackPaused(track *sconsify.Track) {
	gui.updateStatus("Paused: " + track.GetFullTitle())
}

func (gui *Gui) TrackPlaying(track *sconsify.Track) {
	gui.updateStatus("Playing: " + track.GetFullTitle())
}

func (gui *Gui) TrackNotAvailable(track *sconsify.Track) {
	gui.updateTemporaryStatus("Not available: " + track.GetTitle())
}

func (gui *Gui) Shutdown() {
	events.ShutdownEngine()
}

func (gui *Gui) PlayTokenLost() error {
	gui.updateStatus("Play token lost")
	return nil
}

func (gui *Gui) GetNextToPlay() *sconsify.Track {
	if !queue.isEmpty() {
		return gui.getNextFromQueue()
	} else if playlists.HasPlaylistSelected() {
		return gui.getNextFromPlaylist()
	}
	return nil
}

func (gui *Gui) NewPlaylists(newPlaylist sconsify.Playlists) error {
	if playlists == nil {
		playlists = &newPlaylist
		go gui.initGui()
	} else {
		playlists.Merge(&newPlaylist)
		go func() {
			gui.updatePlaylistsView()
			gui.updateTracksView()
			gui.g.Flush()
		}()
	}
	return nil
}

func (gui *Gui) initGui() {
	gui.g = gocui.NewGui()
	if err := gui.g.Init(); err != nil {
		log.Panicln(err)
	}
	defer gui.g.Close()

	gui.g.SetLayout(layout)
	if err := keybindings(); err != nil {
		log.Panicln(err)
	}
	gui.g.SelBgColor = gocui.ColorGreen
	gui.g.SelFgColor = gocui.ColorBlack
	gui.g.ShowCursor = true

	err := gui.g.MainLoop()
	if err != nil && err != gocui.ErrorQuit {
		log.Panicln(err)
	}
}

func (gui *Gui) updateTemporaryStatus(message string) {
	go func() {
		time.Sleep(4 * time.Second)
		gui.updateStatus(gui.currentMessage)
	}()
	gui.setStatus(message)
}

func (gui *Gui) updateStatus(message string) {
	gui.currentMessage = message
	gui.setStatus(message)
}

func (gui *Gui) setStatus(message string) {
	gui.clearStatusView()
	fmt.Fprintf(gui.statusView, playlists.GetModeAsString()+"%v", message)
	// otherwise the update will appear only in the next keyboard move
	gui.g.Flush()
}

func (gui *Gui) getSelectedPlaylist() *sconsify.Playlist {
	playlistName, _ := gui.getSelected(gui.playlistsView)
	if playlistName != "" {
		return playlists.Get(playlistName)
	}
	return nil
}

func (gui *Gui) getSelectedTrack() (string, error) {
	return gui.getSelected(gui.tracksView)
}

func (gui *Gui) getQueueSelectedTrackIndex() int {
	_, cy := gui.queueView.Cursor()
	return cy
}

func (gui *Gui) getSelected(v *gocui.View) (string, error) {
	var l string
	var err error

	_, cy := v.Cursor()
	if l, err = v.Line(cy); err != nil {
		l = ""
	}

	return l, err
}

func (gui *Gui) getNextFromPlaylist() *sconsify.Track {
	track, _ := playlists.GetNext()
	return track
}

func (gui *Gui) getNextFromQueue() *sconsify.Track {
	track := queue.Pop()
	go gui.updateQueueView()
	return track
}

func (gui *Gui) playNext() {
	events.NextPlay()
}

func (gui *Gui) getCurrentSelectedTrack() *sconsify.Track {
	currentPlaylist := gui.getSelectedPlaylist()
	currentTrack, errTrack := gui.getSelectedTrack()
	if currentPlaylist != nil && errTrack == nil {
		if currentPlaylist != nil {
			currentTrack = currentTrack[0:strings.Index(currentTrack, ".")]
			currentIndexTrack, _ := strconv.Atoi(currentTrack)
			currentIndexTrack = currentIndexTrack - 1
			track := currentPlaylist.Track(currentIndexTrack)
			playlists.SetCurrents(currentPlaylist.Name(), currentIndexTrack)
			return track
		}
	}
	return nil
}

func (gui *Gui) updateTracksView() {
	gui.tracksView.Clear()
	gui.tracksView.SetCursor(0, 0)
	gui.tracksView.SetOrigin(0, 0)
	currentPlaylist := gui.getSelectedPlaylist()
	if currentPlaylist != nil {
		for i := 0; i < currentPlaylist.Tracks(); i++ {
			track := currentPlaylist.Track(i)
			fmt.Fprintf(gui.tracksView, "%v. %v", (i + 1), track.GetTitle())
		}
	}
}

func (gui *Gui) updatePlaylistsView() {
	gui.playlistsView.Clear()
	keys := playlists.GetNames()
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintln(gui.playlistsView, key)
	}
}

func (gui *Gui) updateQueueView() {
	gui.queueView.Clear()
	if !queue.isEmpty() {
		for _, track := range queue.Contents() {
			fmt.Fprintf(gui.queueView, "%v", track.GetTitle())
		}
	}
}

func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView(VIEW_PLAYLISTS, -1, -1, 25, maxY-2); err != nil {
		if err != gocui.ErrorUnkView {
			return err
		}
		gui.playlistsView = v
		gui.playlistsView.Highlight = true

		gui.updatePlaylistsView()

		if err := g.SetCurrentView(VIEW_PLAYLISTS); err != nil {
			return err
		}
	}
	if v, err := g.SetView(VIEW_TRACKS, 25, -1, maxX-50, maxY-2); err != nil {
		if err != gocui.ErrorUnkView {
			return err
		}
		gui.tracksView = v

		gui.updateTracksView()
	}
	if v, err := g.SetView(VIEW_QUEUE, maxX-50, -1, maxX, maxY-2); err != nil {
		if err != gocui.ErrorUnkView {
			return err
		}
		gui.queueView = v
	}
	if v, err := g.SetView(VIEW_STATUS, -1, maxY-2, maxX, maxY); err != nil {
		if err != gocui.ErrorUnkView {
			return err
		}
		gui.statusView = v
	}
	return nil
}

func (gui *Gui) enableTracksView() error {
	gui.tracksView.Highlight = true
	gui.playlistsView.Highlight = false
	gui.queueView.Highlight = false
	return gui.g.SetCurrentView(VIEW_TRACKS)
}

func (gui *Gui) enableSideView() error {
	gui.tracksView.Highlight = false
	gui.playlistsView.Highlight = true
	gui.queueView.Highlight = false
	return gui.g.SetCurrentView(VIEW_PLAYLISTS)
}

func (gui *Gui) enableQueueView() error {
	gui.tracksView.Highlight = false
	gui.playlistsView.Highlight = false
	gui.queueView.Highlight = true
	return gui.g.SetCurrentView(VIEW_QUEUE)
}

func (gui *Gui) clearStatusView() {
	gui.statusView.Clear()
	gui.statusView.SetCursor(0, 0)
	gui.statusView.SetOrigin(0, 0)
}
