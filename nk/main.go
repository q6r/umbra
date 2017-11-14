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

type GUI struct {
	c       *core.Core
	targetID   string
	toAddContact      []byte
	chatInput  map[string][]byte
	chatOutput map[string][]byte
}

var gui *GUI = nil

func main() {

	var err error
	gui = &GUI{}
	gui.targetID = ""
	gui.chatInput = make(map[string][]byte)
	gui.chatOutput = make(map[string][]byte)
	gui.toAddContact = make([]byte, 256)

	gui.c, err = core.New(context.Background(), *repoPath)
	if err != nil {
		panic(err)
	}

	err = gui.c.Load()
	if err != nil {
		fmt.Printf("Unable to load program state\n")
	}
	defer func() {
		fmt.Printf("Saving program state\n")
		err := gui.c.Save()
		if err != nil {
			fmt.Printf("Unable to save program state : %#v\n", err)
		}
		err = gui.c.Close()
		if err != nil {
			fmt.Printf("Unable to close program : %#v\n", err)
		}
	}()

	gui.c.Events.On("*", func(event *emitter.Event) {

		if strings.Contains(event.OriginalTopic, "message:recieved") {
			msg, ok := event.Args[0].(floodsub.Message)
			if !ok {
				fmt.Printf("Event is not a message %#v\n", event.Args)
				return
			}

			if _, ok := gui.chatOutput[msg.GetFrom().Pretty()]; !ok {
				gui.chatOutput[msg.GetFrom().Pretty()] = make([]byte, 32000)
			}
			gui.chatOutput[msg.GetFrom().Pretty()] = appenderNewLine(gui.chatOutput[msg.GetFrom().Pretty()],
				[]byte(fmt.Sprintf("<%s:him> %s", time.Now().Format("2006-01-02 15:04:05"), string(msg.GetData()))))
		}

		if strings.Contains(event.OriginalTopic, "message:sent") {

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

	state := &State{}
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

func gfxMain(win *glfw.Window, ctx *nk.Context, state *State) {
	nk.NkPlatformNewFrame()

	width, height := win.GetSize()

	mainEvent := nk.NkBegin(ctx, "Contacts", nk.NkRect(0, 0, float32(width), float32(height)), nk.WindowTitle)
	if mainEvent > 0 {

		nk.NkLayoutRowDynamic(ctx, float32(height), 1)
		{
			// ContactListView
			if len(gui.targetID) == 0 {
				if nk.NkGroupBegin(ctx, "List", 0) > 0 {
					nk.NkLayoutRowDynamic(ctx, 25, 1)
					{
						// Adding contact area
						nk.NkLayoutRowDynamic(ctx, 25, 2)
						{
							if nk.NkEditStringZeroTerminated(ctx, nk.EditField, gui.toAddContact, 256, nk.NkFilterAscii) > 0 {
							}

							if nk.NkButtonLabel(ctx, "+") > 0 {
								err := gui.c.AddContact(strings.TrimRight(string(gui.toAddContact), "\x00"))
								if err != nil {
									fmt.Printf("Unable to add contact %s\n", err.Error())
								}
								gui.toAddContact[0] = 0
							}
						}
						nk.NkLayoutRowDynamic(ctx, 25, 1)
						{
							// List area
							for _, contact := range gui.c.Contacts {	
								if nk.NkButtonLabel(ctx, contact.ID) > 0 {
									gui.targetID = contact.ID
								}
							}
						}
					}
					nk.NkGroupEnd(ctx)
				}
			}

			// ChatView
			if len(gui.targetID) > 0 {
				switch event := nk.NkGroupBegin(ctx, gui.targetID, nk.WindowTitle|nk.WindowMinimizable)
				{
				case event == 1:

					nk.NkLayoutRowDynamic(ctx, 25, 1)
					{
						if nk.NkButtonLabel(ctx, "delete") > 0 {
							err := gui.c.DeleteContact(gui.targetID)
							if err != nil {
								fmt.Printf("Unable to delete contact : %#v\n", err)
							}
							gui.targetID = ""
							// TODO : remove allocate buffers if exists...
						}
					}

					nk.NkLayoutRowDynamic(ctx, float32(height)-100-25-25, 1)
					{
						// initalize buffers if not initialized
						if _, ok := gui.chatOutput[gui.targetID]; !ok {
							gui.chatOutput[gui.targetID] = make([]byte, 32000)
						}
						if nk.NkEditStringZeroTerminated(ctx, nk.EditMultiline, gui.chatOutput[gui.targetID], 32000, nk.NkFilterAscii) > 0 {

						}
					}
					nk.NkLayoutRowDynamic(ctx, 25, 2)
					{
						// initalize buffers if not initialized
						if _, ok := gui.chatInput[gui.targetID]; !ok {
							gui.chatInput[gui.targetID] = make([]byte, 256)
						}
						if nk.NkEditStringZeroTerminated(ctx, nk.EditField, gui.chatInput[gui.targetID], 256, nk.NkFilterAscii) > 0 {

						}
						if nk.NkButtonLabel(ctx, "send") > 0 {
							gui.chatOutput[gui.targetID] = appenderNewLine(gui.chatOutput[gui.targetID],
								[]byte(fmt.Sprintf("<%s:me> %s", time.Now().Format("2006-01-02 15:04:05"), gui.chatInput[gui.targetID])))
							
							// find the contact
							for _, contact := range gui.c.Contacts {
								if contact.ID == gui.targetID {
									ptype := payload.Payload_MSG
									p := payload.Payload{
										Type: &ptype,
										Body: gui.chatInput[gui.targetID],
									}
									err := contact.WriteEncryptedPayload(p)
									if err != nil {
										fmt.Printf("Unable to write message : %#v\n", err.Error())
									} else {
										fmt.Printf("Message sent!\n")
									}
								}
							}

							gui.chatInput[gui.targetID][0] = 0
						}
					}
					nk.NkGroupEnd(ctx)
				case event == nk.WindowMinimized:
					gui.targetID = ""
				default:
					fmt.Printf("event = %d\n", event)
				}
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

type State struct {
}

func onError(code int32, msg string) {
	log.Printf("[glfw ERR]: error %d: %s", code, msg)
}
