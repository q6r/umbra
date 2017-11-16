package main

import (
	"unsafe"
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
	isOnline   map[string]bool
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

var imageOnlineStatusID      = uint32(40)
var imageOfflineStatusID     = uint32(41)

// TODO : glDeleteTextures(1, <textures>);
// initializeStatusImage will builld the online/offline image textures
func initializeStatusImages() {
	imageOnlineStatusData    := make([]byte, 80*80*4)
	imageOnlineStatusDataPtr := unsafe.Pointer(nil)
	i := 0
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			imageOnlineStatusData[i + 0] = 255;
			imageOnlineStatusData[i + 1] = 0;
			imageOnlineStatusData[i + 2] = 255;
			imageOnlineStatusData[i + 3] = 0;
			i += 4;
		}
	}
	imageOnlineStatusDataPtr = unsafe.Pointer(uintptr(unsafe.Pointer(&imageOnlineStatusData[0])) + unsafe.Sizeof(imageOnlineStatusData[0]))

	gl.GenTextures(1, &imageOnlineStatusID)
	gl.BindTexture(gl.TEXTURE_2D, imageOnlineStatusID)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, 80, 80, 0, gl.RGBA, gl.UNSIGNED_BYTE, imageOnlineStatusDataPtr)

	imageOfflineStatusData    := make([]byte, 80*80*4)
	imageOfflineStatusDataPtr := unsafe.Pointer(nil)
	i = 0
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			imageOfflineStatusData[i + 0] = 255;
			imageOfflineStatusData[i + 1] = 255;
			imageOfflineStatusData[i + 2] = 0;
			imageOfflineStatusData[i + 3] = 0;
			i += 4;
		}
	}
	imageOfflineStatusDataPtr = unsafe.Pointer(uintptr(unsafe.Pointer(&imageOfflineStatusData[0])) + unsafe.Sizeof(imageOfflineStatusData[0]))

	gl.GenTextures(1, &imageOfflineStatusID)
	gl.BindTexture(gl.TEXTURE_2D, imageOfflineStatusID)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, 80, 80, 0, gl.RGBA, gl.UNSIGNED_BYTE, imageOfflineStatusDataPtr)
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
			o := []byte(fmt.Sprintf("<%s:him> %s\n", time.Now().Format("2006-01-02 15:04:05"), string(msg.GetData())))
			state.chatOutput[msg.GetFrom().Pretty()] = prepend(o, state.chatOutput[msg.GetFrom().Pretty()])
			return nil
		} else if strings.Contains(event.OriginalTopic, "contact:online") {
			contact, ok := event.Args[0].(*core.Contact)
			if !ok {
				return fmt.Errorf("event is not a contact : %#v", event.Args)
			}
			state.isOnline[contact.ID] = true
		} else if strings.Contains(event.OriginalTopic, "contact:offline") {
			contact, ok := event.Args[0].(*core.Contact)
			if !ok {
				return fmt.Errorf("event is not a contact : %#v", event.Args)
			}
			state.isOnline[contact.ID] = false
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
	state.isOnline     = make(map[string]bool)
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
	glfw.WindowHint(glfw.ContextVersionMajor, 2)
	glfw.WindowHint(glfw.ContextVersionMinor, 0)
	//glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	//glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

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

	initializeStatusImages()

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

func ViewContactList(win *glfw.Window, ctx *nk.Context, state *State) {
	width, _ := win.GetSize()
	statusWidth := float32(0.1)
	addWidth    := float32(0.1)
	onlineImage  := nk.NkImageId(int32(imageOnlineStatusID))
	offlineImage := nk.NkImageId(int32(imageOfflineStatusID))

	if nk.NkGroupBegin(ctx, "List", 0) > 0 {
		// Adding contact area
		nk.NkLayoutRowBegin(ctx, nk.LayoutStatic, 25, 2)
		{
			nk.NkLayoutRowPush(ctx, float32(width)*(1-addWidth)-float32(width)*addWidth)
			if nk.NkEditStringZeroTerminated(ctx, nk.EditField, state.toAddContact, 256, nk.NkFilterAscii) > 0 {
			}

			nk.NkLayoutRowPush(ctx, float32(width)*addWidth)
			{
				if nk.NkButtonLabel(ctx, "+") > 0 {
					err := state.c.AddContact(strings.TrimRight(string(state.toAddContact), "\x00"))
					if err != nil {
						fmt.Printf("Unable to add contact %s\n", err.Error())
					}
					state.toAddContact[0] = 0
				}
			}
		}
		nk.NkLayoutRowEnd(ctx)

		// List area
		nk.NkLayoutRowBegin(ctx, nk.LayoutStatic, 25, 2)
		{
			for _, contact := range state.c.Contacts {	
				nk.NkLayoutRowPush(ctx, float32(width)*statusWidth)
				{
					if state.isOnline[contact.ID] {
						nk.NkImage(ctx, onlineImage)
					} else {
						nk.NkImage(ctx, offlineImage)
					}
				}
				nk.NkLayoutRowPush(ctx, float32(width)*(1-statusWidth)-(float32(width)*statusWidth))
				{
					if nk.NkButtonLabel(ctx, contact.ID) > 0 {
						state.targetID = contact.ID
						state.view = "chat"
					}
				}
			}
		}
		nk.NkLayoutRowEnd(ctx)
	}
	nk.NkGroupEnd(ctx)
}

func ViewChat(win *glfw.Window, ctx *nk.Context, state *State, height float32) {
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
			if nk.NkEditStringZeroTerminated(ctx, nk.EditMultiline,
				state.chatOutput[state.targetID], 32000, nk.NkFilterAscii) > 0 {
			}
		}
		nk.NkLayoutRowDynamic(ctx, 25, 2)
		{
			// initalize buffers if not initialized
			if _, ok := state.chatInput[state.targetID]; !ok {
				state.chatInput[state.targetID] = make([]byte, 256)
			}


			if nk.NkEditStringZeroTerminated(ctx, nk.EditField,
				state.chatInput[state.targetID], 256, nk.NkFilterAscii) > 0 {
			}

			sendEvent := nk.NkButtonLabel(ctx, "send")
			if sendEvent > 0 {
				o := []byte(fmt.Sprintf("<%s:me> %s\n", time.Now().Format("2006-01-02 15:04:05"), state.chatInput[state.targetID]))
				state.chatOutput[state.targetID] = prepend(o, state.chatOutput[state.targetID])
				
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

				for i := 0; i<256;i++ {
					state.chatInput[state.targetID][i] = 0x00
				}
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
				ViewContactList(win, ctx, state)
			case "chat":
				ViewChat(win, ctx, state, float32(height))
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

func prepend(a []byte, b []byte) []byte {

	alen := 0
	for alen = 0; a[alen] != 0; alen++ {

	}
	ares := make([]byte, alen+1)
	i := 0
	for i = 0;i < alen; i++ {
		ares[i] = a[i]
	}
	ares[i] = 0x0a	

	return append(ares, b...)
}

func onError(code int32, msg string) {
	log.Printf("[glfw ERR]: error %d: %s", code, msg)
}
