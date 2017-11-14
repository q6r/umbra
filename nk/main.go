package main

import (
	"github.com/q6r/umbra/core/payload"
	"strings"
	"github.com/olebedev/emitter"
	"context"
	"flag"
	"github.com/q6r/umbra/core"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/go-gl/gl/v3.2-core/gl"
	"github.com/go-gl/glfw/v3.2/glfw"
	"github.com/golang-ui/nuklear/nk"
	"github.com/xlab/closer"
	floodsub "gx/ipfs/QmUUSLfvihARhCxxgnjW4hmycJpPvzNu12Aaz6JWVdfnLg/go-libp2p-floodsub"
)

type State struct {
	c          *core.Core
	targetID   string
	toAddContact      []byte
	chatInput  map[string][]byte
	chatOutput map[string][]byte
	view       string 				// contactList, chat, ...
}

const (
	winWidth  = 400
	winHeight = 500

	maxVertexBuffer  = 512 * 1024
	maxElementBuffer = 128 * 1024
)

var repoPath = flag.String("repo", "/tmp/.ipfs", "The repository path")

func init() {
	runtime.LockOSThread()
	flag.Parse()
}

func processEvents(state *State, event *emitter.Event) error {
		if strings.Contains(event.OriginalTopic, "message:recieved") {
			msg, ok := event.Args[0].(floodsub.Message)
			if !ok {
				return fmt.Errorf("event is not a message : %#v", event.Args)
			}

			if _, ok := state.chatOutput[msg.GetFrom().Pretty()]; !ok {
				state.chatOutput[msg.GetFrom().Pretty()] = make([]byte, 32000)
			}
			state.chatOutput[msg.GetFrom().Pretty()] = appenderNewLine(state.chatOutput[msg.GetFrom().Pretty()],
				[]byte(fmt.Sprintf("<%s:him> %s", time.Now().Format("2006-01-02 15:04:05"), string(msg.GetData()))))
			return nil
		}

		return nil
}

func main() {

	var err error
	state := &State{}
	state.targetID     = ""
	state.view         = "contactList"
	state.chatInput    = make(map[string][]byte)
	state.chatOutput   = make(map[string][]byte)
	state.toAddContact = make([]byte, 256)

	state.c, err = core.New(context.Background(), *repoPath)
	if err != nil {
		panic(err)
	}

	err = state.c.Load()
	if err != nil {
		fmt.Printf("Unable to load program state\n")
	}
	defer func() {
		fmt.Printf("Saving program state\n")
		err := state.c.Save()
		if err != nil {
			fmt.Printf("Unable to save program state : %#v\n", err)
		}
		err = state.c.Close()
		if err != nil {
			fmt.Printf("Unable to close program : %#v\n", err)
		}
	}()

	state.c.Events.On("*", func(event *emitter.Event) {
		err := processEvents(state, event)
		if err != nil {
			fmt.Printf("Event error : %s\n", err.Error())
			return
		}
	})

	if err := glfw.Init(); err != nil {
		closer.Fatalln(err)
	}
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 2)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	win, err := glfw.CreateWindow(winWidth, winHeight, "Umbra", nil, nil)
	if err != nil {
		closer.Fatalln(err)
	}
	win.MakeContextCurrent()

	width, height := win.GetSize()
	log.Printf("glfw: created window %dx%d", width, height)

	if err := gl.Init(); err != nil {
		closer.Fatalln("opengl: init failed:", err)
	}
	gl.Viewport(0, 0, int32(width), int32(height))

	ctx := nk.NkPlatformInit(win, nk.PlatformInstallCallbacks)

	atlas := nk.NewFontAtlas()
	nk.NkFontStashBegin(&atlas)
	sansFont := nk.NkFontAtlasAddFromBytes(atlas, MustAsset("assets/FreeSans.ttf"), 16, nil)
	// sansFont := nk.NkFontAtlasAddDefault(atlas, 16, nil)
	nk.NkFontStashEnd()
	if sansFont != nil {
		nk.NkStyleSetFont(ctx, sansFont.Handle())
	}

	exitC := make(chan struct{}, 1)
	doneC := make(chan struct{}, 1)
	closer.Bind(func() {
		close(exitC)
		<-doneC
	})

	fpsTicker := time.NewTicker(time.Second / 30)
	for {
		select {
		case <-exitC:
			nk.NkPlatformShutdown()
			glfw.Terminate()
			fpsTicker.Stop()
			close(doneC)
			return
		case <-fpsTicker.C:
			if win.ShouldClose() {
				close(exitC)
				continue
			}
			glfw.PollEvents()
			gfxMain(win, ctx, state)
		}
	}
}

var commandBuffer = nk.NewCommandBuffer()
var rect = nk.NewRect()

func ViewContactList(ctx *nk.Context, state *State) {
	if nk.NkGroupBegin(ctx, "List", 0) > 0 {
		nk.NkLayoutRowDynamic(ctx, 25, 1)
		{
			// Adding contact area
			nk.NkLayoutRowDynamic(ctx, 25, 2)
			{
				if nk.NkEditStringZeroTerminated(ctx, nk.EditField, state.toAddContact, 256, nk.NkFilterAscii) > 0 {
				}

				if nk.NkButtonLabel(ctx, "+") > 0 {
					err := state.c.AddContact(strings.TrimRight(string(state.toAddContact), "\x00"))
					if err != nil {
						fmt.Printf("Unable to add contact %s\n", err.Error())
					}
					state.toAddContact[0] = 0
				}
			}
			nk.NkLayoutRowDynamic(ctx, 25, 1)
			{
				// List area
				for _, contact := range state.c.Contacts {	
					if nk.NkButtonLabel(ctx, contact.ID) > 0 {
						state.targetID = contact.ID
						state.view = "chat"
					}
				}
			}
		}
		nk.NkGroupEnd(ctx)
	}
}

func ViewChat(ctx *nk.Context, state *State, height float32) {
	switch event := nk.NkGroupBegin(ctx, state.targetID, nk.WindowTitle|nk.WindowMinimizable)
	{
	case event == 1:

		nk.NkLayoutRowDynamic(ctx, 25, 1)
		{
			if nk.NkButtonLabel(ctx, "delete") > 0 {
				err := state.c.DeleteContact(state.targetID)
				if err != nil {
					fmt.Printf("Unable to delete contact : %#v\n", err)
				}
				state.targetID = ""
				state.view = "contactList"
				// TODO : remove allocate buffers if exists...
			}
		}

		nk.NkLayoutRowDynamic(ctx, float32(height)-100-25-25, 1)
		{
			// initalize buffers if not initialized
			if _, ok := state.chatOutput[state.targetID]; !ok {
				state.chatOutput[state.targetID] = make([]byte, 32000)
			}
			if nk.NkEditStringZeroTerminated(ctx, nk.EditMultiline, state.chatOutput[state.targetID], 32000, nk.NkFilterAscii) > 0 {

			}
		}
		nk.NkLayoutRowDynamic(ctx, 25, 2)
		{
			// initalize buffers if not initialized
			if _, ok := state.chatInput[state.targetID]; !ok {
				state.chatInput[state.targetID] = make([]byte, 256)
			}
			if nk.NkEditStringZeroTerminated(ctx, nk.EditField, state.chatInput[state.targetID], 256, nk.NkFilterAscii) > 0 {

			}
			if nk.NkButtonLabel(ctx, "send") > 0 {
				state.chatOutput[state.targetID] = appenderNewLine(state.chatOutput[state.targetID],
					[]byte(fmt.Sprintf("<%s:me> %s", time.Now().Format("2006-01-02 15:04:05"), state.chatInput[state.targetID])))
				
				// find the contact
				for _, contact := range state.c.Contacts {
					if contact.ID == state.targetID {
						ptype := payload.Payload_MSG
						p := payload.Payload{
							Type: &ptype,
							Body: state.chatInput[state.targetID],
						}
						err := contact.WriteEncryptedPayload(p)
						if err != nil {
							fmt.Printf("Unable to write message : %#v\n", err.Error())
						} else {
							fmt.Printf("Message sent!\n")
						}
					}
				}

				state.chatInput[state.targetID][0] = 0
			}
		}
		nk.NkGroupEnd(ctx)
	case event == nk.WindowMinimized:
		state.targetID = ""
		state.view     = "contactList"
	default:
		fmt.Printf("event = %d\n", event)
	}
}

func gfxMain(win *glfw.Window, ctx *nk.Context, state *State) {
	nk.NkPlatformNewFrame()

	width, height := win.GetSize()

	if nk.NkBegin(ctx, "Contacts", nk.NkRect(0, 0, float32(width), float32(height)), nk.WindowTitle) > 0 {
		nk.NkLayoutRowDynamic(ctx, float32(height), 1)
		{

			switch state.view {
			case "contactList":
				ViewContactList(ctx, state)
			case "chat":
				ViewChat(ctx, state, float32(height))
			}
		}
	}
	nk.NkEnd(ctx)

	// Render
	bg := make([]float32, 4)
	width, height = win.GetSize()
	gl.Viewport(0, 0, int32(width), int32(height))
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.ClearColor(bg[0], bg[1], bg[2], bg[3])
	nk.NkPlatformRender(nk.AntiAliasingOn, maxVertexBuffer, maxElementBuffer)
	win.SwapBuffers()
}

func appenderNewLine(a []byte, b []byte) []byte {
	return appender(appender(a, b), []byte{0x0a})
}

func appender(a []byte, b []byte) []byte {
	i := 0
	for i = 0; i < len(a); i++ {
		if a[i] == 0 {
			break;
		}
	}
	if i >= len(a) { // increase memory of a
		return a
	}
	return append(a[:i], b...)[0:len(a)]
}

func onError(code int32, msg string) {
	log.Printf("[glfw ERR]: error %d: %s", code, msg)
}
