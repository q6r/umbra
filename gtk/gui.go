package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	floodsub "github.com/libp2p/go-floodsub"
	"github.com/mattn/go-gtk/gdk"
	"github.com/olebedev/emitter"
	"github.com/q6r/umbra/core"
	"github.com/q6r/umbra/core/payload"

	"github.com/mattn/go-gtk/gdkpixbuf"
	"github.com/mattn/go-gtk/glib"
	"github.com/mattn/go-gtk/gtk"
)

// chatBuffer is used to keep record of [contact.ID] -> chat buffer
var chatBuffer = make(map[string]*gtk.TextBuffer)

var repoPath = flag.String("repo", "/tmp/.ipfs", "The repository path")

func init() {
	flag.Parse()
}

func main() {
	runtime.LockOSThread()

	ctx, cancel := context.WithCancel(context.Background())
	c, err := core.New(ctx, *repoPath)
	if err != nil {
		panic(err)
	}
	defer cancel()
	defer func() { // TODO : handle errors
		err := c.Save()
		if err != nil {
			fmt.Printf("Unable to save core state\n")
		}
	}()
	defer c.Close() // TODO : handle errors

	gtk.Init(&os.Args)
	window := gtk.NewWindow(gtk.WINDOW_TOPLEVEL)
	window.SetTitle("Umbra")
	window.Connect("destroy", gtk.MainQuit)

	//--------------------------------
	// VBOX
	//--------------------------------
	vbox := gtk.NewVBox(false, 1)

	//--------------------------------
	// Menubar
	//--------------------------------
	menubar := createMainMenubar(c)
	vbox.PackStart(menubar, false, false, 0)

	//--------------------------------
	// Contacts
	//--------------------------------
	vpaned := gtk.NewVPaned()
	vbox.Add(vpaned)

	contactList := gtk.NewScrolledWindow(nil, nil)
	vpaned.Add(contactList)

	//--------------------------------
	// Contacts Store
	//--------------------------------
	contactStore := gtk.NewTreeStore(gdkpixbuf.GetType(), glib.G_TYPE_STRING)
	treeview := gtk.NewTreeView()
	contactList.Add(treeview)

	treeview.SetModel(contactStore.ToTreeModel())
	treeview.AppendColumn(gtk.NewTreeViewColumnWithAttributes("List", gtk.NewCellRendererText(), "text", 1))

	c.Events.On("*", func(event *emitter.Event) {
		if strings.Contains(event.OriginalTopic, "contact:") {
			updateContactStore(c, contactStore)
		} else if strings.Contains(event.OriginalTopic, "message:recieved") {
			msg := event.Args[0].(floodsub.Message) // TODO : check casting if ok
			outMsg := fmt.Sprintf("%s : %s\n", msg.GetFrom().Pretty(), string(msg.GetData()))

			buffer, ok := chatBuffer[msg.GetFrom().Pretty()]
			if !ok {
				fmt.Printf("Error : not chat buffer found!!!!\n")
				return
			}

			// update the chat buffer
			fmt.Printf("Updating the buffer %#v\n", buffer)
			var tvIter gtk.TextIter
			buffer.GetEndIter(&tvIter)
			buffer.Insert(&tvIter, outMsg)

			// TODO : set notification

		} else {
			fmt.Printf("event = %#v\n", event)
		}
	})
	err = c.Load()
	if err != nil {
		fmt.Printf("Unable to reload core : %s", err.Error())
	}
	updateContactStore(c, contactStore)

	treeview.Connect("row_activated", func() {
		var path *gtk.TreePath
		var column *gtk.TreeViewColumn
		treeview.GetCursor(&path, &column)

		contactIndex, err := strconv.Atoi(path.String())
		if err != nil {
			panic(err)
		}
		contact := c.Contacts[contactIndex]

		createChatWindow(window, c, contact)
	})

	// end
	window.Add(vbox)
	window.SetSizeRequest(400, 200)
	window.ShowAll()

	gtk.Main()
}

func createMainMenubar(c *core.Core) *gtk.MenuBar {
	menubar := gtk.NewMenuBar()
	fileMenuItem := gtk.NewMenuItemWithMnemonic("_File")
	menubar.Append(fileMenuItem)

	fileSubMenuItem := gtk.NewMenu()
	fileMenuItem.SetSubmenu(fileSubMenuItem)

	fileAddContactSubMenuItem := gtk.NewMenuItemWithMnemonic("_Add")
	fileAddContactSubMenuItem.Connect("activate", func() {
		createAddContactWindow(c)
	})
	fileSubMenuItem.Append(fileAddContactSubMenuItem)

	fileExitSubMenuItem := gtk.NewMenuItemWithMnemonic("E_xit")
	fileExitSubMenuItem.Connect("activate", func() {
		gtk.MainQuit()
	})
	fileSubMenuItem.Append(fileExitSubMenuItem)
	return menubar
}

func createChatWindowMenubar(parent *gtk.Window, c *core.Core, contact *core.Contact) *gtk.MenuBar {
	menubar := gtk.NewMenuBar()
	optionMenuItem := gtk.NewMenuItemWithMnemonic("_Option")
	menubar.Append(optionMenuItem)

	optionSubMenuItem := gtk.NewMenu()
	optionMenuItem.SetSubmenu(optionSubMenuItem)

	optionAddContactSubMenuItem := gtk.NewMenuItemWithMnemonic("_Delete")
	optionAddContactSubMenuItem.Connect("activate", func() {
		fmt.Printf("Deleting %#v\n", contact)
		err := c.DeleteContact(contact.ID)
		if err != nil {
			fmt.Printf("Unable to delete contact : %#v", err)
			return
		}
		fmt.Printf("Contact %s deleted\n", contact.ID)
		delete(chatBuffer, contact.ID)
		parent.Destroy()
	})
	optionSubMenuItem.Append(optionAddContactSubMenuItem)

	optionCloseContactSubMenuItem := gtk.NewMenuItemWithMnemonic("_Close")
	optionCloseContactSubMenuItem.Connect("activate", func() {
		parent.Destroy()
	})
	optionSubMenuItem.Append(optionCloseContactSubMenuItem)

	return menubar
}

// createChatWindow
/*
- Window             4/4
    - Menubar        4/4
	- VBOX           4/4
		- TextOutput 4/4
		- HPaned     4/4
			- Entry  3/4
			- Button 1/4
*/
func createChatWindow(window *gtk.Window, c *core.Core, contact *core.Contact) {
	chatWindow := gtk.NewWindow(gtk.WINDOW_TOPLEVEL)
	chatWindow.SetTitle(contact.ID)
	chatWindow.SetTypeHint(gdk.WINDOW_TYPE_HINT_DIALOG)
	chatWindow.SetPosition(gtk.WIN_POS_CENTER)

	vbox := gtk.NewVBox(false, 1)
	chatWindow.Add(vbox)

	menubar := createChatWindowMenubar(chatWindow, c, contact)
	vbox.PackStart(menubar, false, false, 0)

	vpaned := gtk.NewVPaned()
	vbox.Add(vpaned)

	textViewBuffer, ok := chatBuffer[contact.ID]
	if !ok {
		fmt.Printf("FATAL ERROR : not chatbufefr for %#v", contact.ID)
	}

	textView := gtk.NewTextViewWithBuffer(*textViewBuffer)
	textView.SetEditable(false)
	textView.SetSizeRequest(680, 480)

	scrolledTextView := gtk.NewScrolledWindow(nil, nil)
	scrolledTextView.Add(textView)
	vpaned.Add(scrolledTextView)

	hpaned := gtk.NewHPaned()
	vpaned.Add(hpaned)

	textEntry := gtk.NewEntry()
	textEntry.SetEditable(true)
	textEntryW := int(0)
	textEntryH := int(0)
	textEntry.GetSizeRequest(&textEntryW, &textEntryH)
	textEntry.SetSizeRequest(680-80, textEntryH)
	hpaned.Add(textEntry)

	sendButton := gtk.NewButtonWithMnemonic("_send")
	sendButton.Connect("clicked", func() {
		var tvIter gtk.TextIter
		textViewBuffer.GetEndIter(&tvIter)

		// create payload to send
		ptype := payload.Payload_MSG
		p := payload.Payload{
			Type: &ptype,
			Body: []byte(textEntry.GetText()),
		}
		err := contact.WriteEncryptedPayload(p)
		if err != nil {
			textViewBuffer.Insert(&tvIter, fmt.Sprintf("Error : %s\n", err.Error()))
			textEntry.SetText("")
			return
		}

		textViewBuffer.Insert(&tvIter, fmt.Sprintf("%s : %s\n", c.Node.Identity.Pretty(), textEntry.GetText()))
		textEntry.SetText("")
	})
	hpaned.Add(sendButton)

	chatWindow.ShowAll()
}

// createAddContactWindow
// TODO : align buttons to be of same size
// TODO : set default focus on add button
func createAddContactWindow(c *core.Core) {
	dialog := gtk.NewDialog()
	dialog.SetTitle("Add")

	vbox := dialog.GetVBox()

	label := gtk.NewLabel("ID")
	vbox.Add(label)

	idEntry := gtk.NewEntry()
	idEntry.SetEditable(true)
	vbox.Add(idEntry)

	hpaned := gtk.NewHPaned()
	vbox.Add(hpaned)

	buttonAdd := gtk.NewButtonWithLabel("Add")
	buttonAdd.Connect("clicked", func() {
		id := idEntry.GetText()

		err := c.AddContact(id) // TODO : handle error
		if err != nil {
			fmt.Printf("Error while adding contact : %s\n", err.Error())
		}

		dialog.Destroy()
	})
	hpaned.Add(buttonAdd)

	buttonCancel := gtk.NewButtonWithLabel("Cancel")
	buttonCancel.Connect("clicked", func() {
		dialog.Destroy()
	})
	hpaned.Add(buttonCancel)

	dialog.ShowAll()
}

// updateContactStore this updates the list store
// also initialize the contact chat buffer if not initialized
// so we can have a history...
func updateContactStore(c *core.Core, contactStore *gtk.TreeStore) {
	contactStore.Clear()
	for _, contact := range c.Contacts {

		// if the contact doesn't have a chat buffer create one!
		_, ok := chatBuffer[contact.ID]
		if !ok {
			textViewTagTable := gtk.NewTextTagTable()
			textViewBuffer := gtk.NewTextBuffer(textViewTagTable)
			chatBuffer[contact.ID] = textViewBuffer
		}

		var iter1 gtk.TreeIter
		contactStore.Append(&iter1, nil)
		contactStore.Set(&iter1, nil, contact.ID)
	}
}