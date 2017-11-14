package main

import (
	"errors"
	"github.com/olebedev/emitter"
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/mattn/go-gtk/gdk"
	"github.com/q6r/umbra/core"
	"github.com/q6r/umbra/core/payload"

	"github.com/mattn/go-gtk/gdkpixbuf"
	"github.com/mattn/go-gtk/glib"
	"github.com/mattn/go-gtk/gtk"

	floodsub "gx/ipfs/QmUUSLfvihARhCxxgnjW4hmycJpPvzNu12Aaz6JWVdfnLg/go-libp2p-floodsub"
)

// chatBuffer is used to keep record of [contact.ID] -> chat buffer
var chatBuffer = make(map[string]*gtk.TextBuffer)
var contactStatus = make(map[string]bool)

var repoPath = flag.String("repo", "/tmp/.ipfs", "The repository path")

func init() {
	runtime.LockOSThread()
	flag.Parse()
}

func processContactAdd(event *emitter.Event, c *core.Core, contactStore *gtk.TreeStore) error {
	fmt.Printf("contact:add\n")
	contact, ok := event.Args[0].(*core.Contact)
	if !ok {
		return fmt.Errorf("Erro event is not a contact : %#v\n", event.Args[0])
	}

	// if the contact doesn't have a chat buffer create one!
	_, ok = chatBuffer[contact.ID]
	if !ok {
		fmt.Printf("No chat buffer found\n creating one\n")
		chatBuffer[contact.ID] = gtk.NewTextBuffer(gtk.NewTextTagTable())
	}

	updateContactStore(c, contactStore) // has ui effect

	return nil
}

func processContactDelete(event *emitter.Event, c *core.Core, contactStore *gtk.TreeStore) error {
	contact, ok := event.Args[0].(*core.Contact)
	if !ok {
		return fmt.Errorf("Erro event is not a contact : %#v\n", event.Args[0])
	}

	if contact == nil {
		return fmt.Errorf("nil contact passed\n")
	}

	delete(chatBuffer, contact.ID)

	updateContactStore(c, contactStore) // has ui effect!

	return nil
}

func processMessageRecieved(event *emitter.Event, c *core.Core) error {
	msg, ok := event.Args[0].(floodsub.Message)
	if !ok {
		return fmt.Errorf("Error event is not a message : %#v\n", event.Args[0])
	}
	outMsg := fmt.Sprintf("%s : %s\n", msg.GetFrom().Pretty(), string(msg.GetData()))

	// update the chat buffer
	buffer, ok := chatBuffer[msg.GetFrom().Pretty()]
	if !ok {
		return fmt.Errorf("Error : not chat buffer found!!!!\n")
	}
	fmt.Printf("Updating the buffer %#v\n", buffer)
	var tvIter gtk.TextIter
	buffer.GetEndIter(&tvIter)
	buffer.Insert(&tvIter, outMsg)

	// TODO : set notification
	return nil
}

func processContactStatus(event *emitter.Event, c *core.Core, contactStore *gtk.TreeStore) error {
	contact, ok := event.Args[0].(*core.Contact)
	if !ok {
		return fmt.Errorf("Error event is not a contact : %#v\n", event.Args[0])
	}
	if contact == nil {
		return fmt.Errorf("Error Contact provided is nil!\n")
	}

	status := strings.Contains(event.OriginalTopic, "contact:online")
	if contactStatus[contact.ID] != status {
		contactStatus[contact.ID] = status
		updateContactStore(c, contactStore) // has ui effect
	}

	return nil
}

// TODO : handle errors
func processEvent(event *emitter.Event, c *core.Core, contactStore *gtk.TreeStore) error {
		fmt.Printf("Processing event %#v\n", event)

		if strings.Contains(event.OriginalTopic, "contact:add") {
			return processContactAdd(event, c, contactStore)
		} else if strings.Contains(event.OriginalTopic, "contact:delete") {
			return processContactDelete(event, c, contactStore)
		} else if strings.Contains(event.OriginalTopic, "message:recieved") {
			return processMessageRecieved(event, c)
		} else if strings.Contains(event.OriginalTopic, "contact:online") || strings.Contains(event.OriginalTopic, "contact:offline") {
			return processContactStatus(event, c, contactStore)
		}

		return errors.New("Unhandled event")
}

func main() {
	ctx, _ := context.WithCancel(context.Background())
	c, err := core.New(ctx, *repoPath)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := c.Save()
		if err != nil {
			fmt.Printf("Unable to save state\n")
		}

		//cancel()

		err = c.Close()
		if err != nil {
			fmt.Printf("Unable to closed core\n")
		}
	}()

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
	menubar := createMainMenubar(window, c)
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
	treeview.AppendColumn(gtk.NewTreeViewColumnWithAttributes("Status", gtk.NewCellRendererPixbuf(), "pixbuf", 0))
	treeview.AppendColumn(gtk.NewTreeViewColumnWithAttributes("List", gtk.NewCellRendererText(), "text", 1))

	treeview.Connect("row_activated", func() {
		var path *gtk.TreePath
		var column *gtk.TreeViewColumn
		treeview.GetCursor(&path, &column)

		contactIndex, err := strconv.Atoi(path.String())
		if err != nil {
			fmt.Printf("Unable to convert '%#v' to integer\n", path.String())
			return
		}
		contact := c.Contacts[contactIndex]

		createChatWindow(window, c, contact)
	})

	// end
	window.Add(vbox)
	window.SetSizeRequest(400, 200)
	window.ShowAll()


	// Run the event catcher threads
	// When idle get events and process
	go glib.IdleAdd(func() bool {
		event := <-c.Events.Once("*")
		err := processEvent(&event, c, contactStore)
		if err != nil {
			fmt.Printf("Failed while processing event %#v\n", err)
		}
		fmt.Printf("Processed %#v\n", event)
		return true
	})

	err = c.Load()
	if err != nil {
		fmt.Printf("Unable to reload core : %s", err.Error())
	}
	for _, contact := range c.Contacts {
		chatBuffer[contact.ID] = gtk.NewTextBuffer(gtk.NewTextTagTable())
	}
	updateContactStore(c, contactStore)

	gtk.Main()
}

func createMainMenubar(parent *gtk.Window, c *core.Core) *gtk.MenuBar {
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
		parent.Destroy()
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
		err := c.DeleteContact(contact.ID)
		if err != nil {
			fmt.Printf("Error : Unable to remove contact : %#v\n", err)
			return
		}
		delete(chatBuffer, contact.ID)
		parent.Destroy()
		// TODO : Shouldn't update the contactStore
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
		fmt.Printf("FATAL ERROR : no chat buffer for %#v", contact.ID)
		chatBuffer[contact.ID] = gtk.NewTextBuffer(gtk.NewTextTagTable())
	}
	textViewBuffer = chatBuffer[contact.ID]

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

		textViewBuffer, ok = chatBuffer[contact.ID]
		if !ok {
			fmt.Printf("Trying to send into an invalid buffer\n")
			return
		}

		// create payload to send
		ptype := payload.Payload_MSG
		p := payload.Payload{
			Type: &ptype,
			Body: []byte(textEntry.GetText()),
		}
		err := contact.WriteEncryptedPayload(p)
		if err != nil {
			fmt.Printf("Error while sending message : %#v\n", err)
			textViewBuffer.Insert(&tvIter, fmt.Sprintf("Error : %s\n", err.Error()))
		} else {
			textViewBuffer.Insert(&tvIter, fmt.Sprintf("%s : %s\n", c.Node.Identity.Pretty(), textEntry.GetText()))
		}
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

		stock_id := gtk.STOCK_DISCONNECT
		status, ok := contactStatus[contact.ID];
		if ok && status == true {
			stock_id = gtk.STOCK_CONNECT
		}

		var iter1 gtk.TreeIter
		contactStore.Append(&iter1, nil)
		contactStore.Set(&iter1,
			gtk.NewImage().RenderIcon(stock_id, gtk.ICON_SIZE_SMALL_TOOLBAR, "").GPixbuf,
			contact.ID)
	}
}