package core

import (
	floodsub "gx/ipfs/QmUUSLfvihARhCxxgnjW4hmycJpPvzNu12Aaz6JWVdfnLg/go-libp2p-floodsub"
	peer "gx/ipfs/QmXYjuNuxVzXKJCfWasQk1RqkhVLDM9jtUKhqc2WPQmFSB/go-libp2p-peer"
	"fmt"
	"crypto/sha1"
	"github.com/gtank/cryptopasta"
	"io"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"github.com/q6r/umbra/core/payload"
	"crypto/sha256"
	"encoding/hex"
	"github.com/golang/protobuf/proto"
	"context"
)

// Contact
type Contact struct {
	parent   *Core  // reference to parent
	ID       string `json:"id"`
	Name     string `json:"name"`
	topicIn  string    // where we read
	topicOut string    // where we write
	subscription *floodsub.Subscription
	incommingMessages chan floodsub.Message
}

// NewContact create a new contact
func NewContact(parent *Core, hisID string) (*Contact, error) {
	var err error

	contact := &Contact{}
	contact.parent = parent
	contact.ID = hisID

	parentID := contact.parent.Node.Identity.Pretty()

	//hasher := md5.New()
	hasher := sha256.New()

	hasher.Write([]byte(fmt.Sprintf("from:%s,to:%s", parentID, contact.ID)))
	contact.topicOut = hex.EncodeToString(hasher.Sum(nil))
	hasher.Reset()

	hasher.Write([]byte(fmt.Sprintf("from:%s,to:%s", contact.ID, parentID)))
	contact.topicIn  = hex.EncodeToString(hasher.Sum(nil))
	hasher.Reset()

	contact.subscription, err = contact.parent.Node.Floodsub.Subscribe(contact.topicIn)
	if err != nil {
		return nil, err
	}
	contact.parent.Events.Emit("subscribed", contact.topicIn)

	ctx, cancel := context.WithCancel(context.Background())
	contact.incommingMessages = make(chan floodsub.Message, 256)
	go func() {
		// TODO : Maybe attempt to read again ???
		err := contact.readerPayload(ctx)
		if err != nil {
			cancel()
			//close(contact.incommingMessages) 
			// TODO : what else we doo ???
		}
		return
	}()

	return contact, nil
}

// CreateEncryptedMessage
func (c *Contact) CreateEncryptedMessage(data []byte) (encryptedAesKey []byte, cipherMessage []byte, err error) {

	// Attempt to get contact public key
	// to encrypt the AES symmetric key
	pubkey, err := c.PublicKey()
	if err != nil {
		return []byte{}, []byte{}, err
	}

	// generate random AES key
	aeskey := [32]byte{}
	_, err = io.ReadFull(rand.Reader, aeskey[:])
	if err != nil {
		return []byte{}, []byte{}, err
	}

	// encrypted aes key
	var ekey []byte = aeskey[:]
	encryptedAesKey, err = rsa.EncryptOAEP(sha1.New(), rand.Reader, pubkey, ekey, []byte{})
	if err != nil {
		return []byte{}, []byte{}, err
	}

	// encrypted body with aes
	cipherMessage, err = cryptopasta.Encrypt(data, &aeskey)
	if err != nil {
		return []byte{}, []byte{}, err
	}
 
	return encryptedAesKey, cipherMessage, nil
}

// PublicKey of the contact
func (c *Contact) PublicKey() (*rsa.PublicKey, error) {
	return c.parent.GetPeerPublicRSAKey(context.Background(), c.ID)
}

func (c *Contact) ConnectedOutPeers() []peer.ID {
	return c.parent.Node.Floodsub.ListPeers(c.topicOut)
}

func (c *Contact) Close() {
	c.subscription.Cancel()
	close(c.incommingMessages)
}

// IsOnline check if the contact's id is
// subscribed to our topicOut
func (c *Contact) IsOnline() bool {
	connectedPeers := c.ConnectedOutPeers()
	for _, peer := range connectedPeers {
		if peer.Pretty() == c.ID {
			return true
		}
	}

	return false
}

func (c *Contact) readerPayload(ctx context.Context) error {
	for {
		msg, err := c.subscription.Next(ctx)
		if err != nil { // TODO : must fail in a better way in-
						// case ctx is canceled recursivly from parents
			return err
		}

		if msg == nil {
			return errors.New("empty message")
		}

		// Ignore messages not comming from our contact's ID
		// (eg: someone publishing on the same topic)
		if msg.GetFrom().Pretty() != c.ID {
			continue
		}

		p := &payload.Payload{}
		err = proto.Unmarshal(msg.Data, p)
		if err != nil {
			continue
		}

		// now handle the payload commands
		switch p.GetType() {
		case payload.Payload_MSG:
			cipherText      := p.GetBody()
			encryptedAesKey := p.GetKey()

			plaintext, err := c.parent.Decrypt(encryptedAesKey, cipherText)
			if err != nil {
				continue
			}

			msg.Data = plaintext
			c.parent.Events.Emit("message:recieved", *msg)
			c.incommingMessages <- *msg
		default:
			// do nothing
		}
	}
}

func (c *Contact) Read() chan floodsub.Message {
	return c.incommingMessages
}

func (c *Contact) WriteEncryptedPayload(p payload.Payload) error {
	// encrypt the content
	encryptedAesKey, ciphertext, err := c.CreateEncryptedMessage(p.GetBody())
	if err != nil {
		return err
	}

	p.Body = ciphertext
	p.Key  = encryptedAesKey

	return c.WritePayload(p)
}

func (c *Contact) WritePayload(p payload.Payload) error {
	data, err := proto.Marshal(&p)
	if err != nil {
		return err
	}
	return c.write(data)
}

func (c *Contact) write(data []byte) error {
	// TODO : assert topic is valid ???
	c.parent.Events.Emit("message:sent", data)
	return c.parent.Node.Floodsub.Publish(c.topicOut, data)
}

// Info returns the user info by querying dht
// maybe fill the user information when creating the contact
// such as public-key...etc
func (c *Contact) Info() error {
	return nil
}