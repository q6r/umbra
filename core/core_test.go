package core

import (
	"github.com/olebedev/emitter"
	"crypto/sha1"
	"crypto/rand"
	"crypto/rsa"
	"time"
	"github.com/q6r/umbra/core/payload"
	"context"
	"testing"
	. "github.com/franela/goblin"
	"github.com/golang/protobuf/proto"
)

var pmsgtype = payload.Payload_PAYLOAD_TYPE(payload.Payload_MSG)
var c1body = []byte("hello from c1")
var c2body = []byte("hello from c2")
var c1payload = &payload.Payload{
	Type: &pmsgtype,
	Body: c1body,
}

var c2payload = &payload.Payload{
	Type: &pmsgtype,
	Body: c2body,
}

func TestEncryption(t *testing.T) {
	g := Goblin(t)

	g.Describe("Encryption", func() {

		g.It("Can self encrypt, and self decrypt", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			// Extract node private, public rsa keys
			priv, err := c1.extractSelfRSAPrivateKey()
			g.Assert(err).Equal(nil)

			for i := 0; i < 10; i++ {
				// EncryptOAEP
				secretMessage := []byte("hello world")
				cipherMessage, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, &priv.PublicKey, secretMessage, []byte{})
				g.Assert(err).Equal(nil)

				// attempt to decrypt message using private key
				plainMessage, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, priv, cipherMessage, []byte{})
				g.Assert(err).Equal(nil)
				g.Assert(secretMessage).Equal(plainMessage)
			}

		})

		g.It("Contacts can query each other public keys", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			c2ctx, c2cancel := context.WithCancel(context.Background())
			c2, err := New(c2ctx, "/tmp/.ipfs_test_2")
			g.Assert(err == nil).Equal(true)
			g.Assert(c2 != nil).Equal(true)
			defer c2cancel()
			defer c2.Close()

			err = c1.AddContact(c2.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)

			err = c2.AddContact(c1.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)

			time.Sleep(time.Second * 20)

			c2pub, err := c1.Contacts[0].PublicKey()
			g.Assert(err).Equal(nil)
			g.Assert(c2pub != nil).Equal(true)

			c1pub, err := c2.Contacts[0].PublicKey()
			g.Assert(err).Equal(nil)
			g.Assert(c1pub != nil).Equal(true)

			// Assert that what we have in cores is
			// the same as the extracted keys
			g.Assert(c1.PrivateKey.PublicKey).Equal(*c1pub)
			g.Assert(c2.PrivateKey.PublicKey).Equal(*c2pub)
		})


		g.It("Contacts can query, then encrypt and decrypt messages using their keys", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			c2ctx, c2cancel := context.WithCancel(context.Background())
			c2, err := New(c2ctx, "/tmp/.ipfs_test_2")
			g.Assert(err == nil).Equal(true)
			g.Assert(c2 != nil).Equal(true)
			defer c2cancel()
			defer c2.Close()

			err = c1.AddContact(c2.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)

			err = c2.AddContact(c1.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)

			time.Sleep(time.Second * 20)

			c2pub, err := c1.GetPeerPublicRSAKey(c1ctx, c1.Contacts[0].ID)
			g.Assert(err).Equal(nil)
			g.Assert(c2pub != nil).Equal(true)

			c1pub, err := c1.GetPeerPublicRSAKey(c2ctx, c2.Contacts[0].ID)
			g.Assert(err).Equal(nil)
			g.Assert(c1pub != nil).Equal(true)

			// Assert that what we have in cores is the same as the extracted
			// keys
			g.Assert(c1.PrivateKey.PublicKey).Equal(*c1pub)
			g.Assert(c2.PrivateKey.PublicKey).Equal(*c2pub)

			// c1 encrypt message with c2pub
			secretMessage := []byte("hello world")
			cipherMessage, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, c2pub, secretMessage, []byte{})
			g.Assert(err).Equal(nil)
			// c2 decrypts the message given by c1 which was encrypted with c2's public key
			plainMessage, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, c2.PrivateKey, cipherMessage, []byte{})
			g.Assert(err).Equal(nil)
			g.Assert(secretMessage).Equal(plainMessage)

			// c2 encrypt message with c1pub
			secretMessage = []byte("hello world")
			cipherMessage, err = rsa.EncryptOAEP(sha1.New(), rand.Reader, c1pub, secretMessage, []byte{})
			g.Assert(err).Equal(nil)
			// c1 decrypts the message given by c2 which was encrypted with c1's public key
			plainMessage, err = rsa.DecryptOAEP(sha1.New(), rand.Reader, c1.PrivateKey, cipherMessage, []byte{})
			g.Assert(err).Equal(nil)
			g.Assert(secretMessage).Equal(plainMessage)

		})
	})
}

func TestSimple(t *testing.T) {
	g := Goblin(t) 

	g.Describe("Core", func() {

		g.It("Creates one", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()
		})

		g.It("Creates multiple", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			c2ctx, c2cancel := context.WithCancel(context.Background())
			c2, err := New(c2ctx, "/tmp/.ipfs_test_2")
			g.Assert(err == nil).Equal(true)
			g.Assert(c2 != nil).Equal(true)
			defer c2cancel()
			defer c2.Close()
		})

		g.It("Can add each other", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			c2ctx, c2cancel := context.WithCancel(context.Background())
			c2, err := New(c2ctx, "/tmp/.ipfs_test_2")
			g.Assert(err == nil).Equal(true)
			g.Assert(c2 != nil).Equal(true)
			defer c2cancel()
			defer c2.Close()

			err = c1.AddContact(c2.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)

			err = c2.AddContact(c1.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)
		})
	})
}

func TestOnline(t *testing.T) {
	g := Goblin(t)
	g.Describe("Online Status", func() {
		g.It("It can query status of contacts", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			//defer c1.Close()

			c2ctx, c2cancel := context.WithCancel(context.Background())
			c2, err := New(c2ctx, "/tmp/.ipfs_test_2")
			g.Assert(err == nil).Equal(true)
			g.Assert(c2 != nil).Equal(true)
			defer c2cancel()
			//defer c2.Close()


			err = c1.AddContact(c2.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)

			// c2 is offline ?? because he didn't add us
			status := c1.Contacts[0].IsOnline()
			g.Assert(status).Equal(false)

			err = c2.AddContact(c1.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)

			time.Sleep(time.Second * 12) // delay so they can communicate

			// Since both added each other both are online
			status = c1.Contacts[0].IsOnline()
			g.Assert(status).Equal(true)
			status = c2.Contacts[0].IsOnline()
			g.Assert(status).Equal(true)
		})

		g.It("Can see sub peers", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			c2ctx, c2cancel := context.WithCancel(context.Background())
			c2, err := New(c2ctx, "/tmp/.ipfs_test_2")
			g.Assert(err == nil).Equal(true)
			g.Assert(c2 != nil).Equal(true)
			defer c2cancel()
			defer c2.Close()

			err = c1.AddContact(c2.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)

			p1 := c1.Contacts[0].ConnectedOutPeers()
			g.Assert(len(p1) == 0).Equal(true)


			err = c2.AddContact(c1.Node.Identity.Pretty())
			g.Assert(err == nil).Equal(true)

			time.Sleep(time.Second * 20)

			p1 = c1.Contacts[0].ConnectedOutPeers()
			g.Assert(len(p1) > 0).Equal(true)
		})
	})
}

func TestEvents(t *testing.T) {
	g := Goblin(t)
	g.Describe("Core events", func() {

		g.It("Contact add event", func(done Done) {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_add_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			c1.Events.On("contact:add", func(event *emitter.Event) {
				g.Assert(len(event.Args) > 0).Equal(true)
				add, ok := event.Args[0].(*Contact)
				g.Assert(ok).Equal(true)
				g.Assert(add != nil).Equal(true)
				done()
			})

			err = c1.AddContact("1234")
			g.Assert(err == nil).Equal(true)
		})

		g.It("Contact delete event", func(done Done) {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_delete_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			c1.Events.On("contact:delete", func(event *emitter.Event) {
				g.Assert(len(event.Args) > 0).Equal(true)
				del, ok := event.Args[0].(*Contact)
				g.Assert(ok).Equal(true)
				g.Assert(del != nil).Equal(true)
				done()
			})

			err = c1.AddContact("1234")
			g.Assert(err == nil).Equal(true)

			err = c1.DeleteContact("1234")
			g.Assert(err == nil).Equal(true)
		})

	})
}

func TestSaveReload(t *testing.T) {
	g := Goblin(t)
	g.Describe("SavingReload", func() {

		g.It("Can Save, and load state", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err == nil).Equal(true)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			err = c1.AddContact("a")
			g.Assert(err == nil).Equal(true)

			err = c1.AddContact("b")
			g.Assert(err == nil).Equal(true)

			err = c1.AddContact("c")
			g.Assert(err == nil).Equal(true)

			err = c1.AddContact("d")
			g.Assert(err == nil).Equal(true)

			err = c1.Save()
			g.Assert(err == nil).Equal(true)

			// now remove contacts from core
			err = c1.DeleteContact("a")
			g.Assert(err == nil).Equal(true)

			err = c1.DeleteContact("b")
			g.Assert(err == nil).Equal(true)

			err = c1.DeleteContact("c")
			g.Assert(err == nil).Equal(true)

			err = c1.DeleteContact("d")
			g.Assert(err == nil).Equal(true)

			g.Assert(len(c1.Contacts)).Equal(0)

			// now reload the state of core
			err = c1.Load()
			g.Assert(err == nil).Equal(true)
			g.Assert(len(c1.Contacts) > 0).Equal(true)

		})

	})
}

func TestPayloads(t *testing.T) {
	g := Goblin(t)
	g.Describe("Payloads", func() {

		g.It("Can marshal", func() {
			ctype := payload.Payload_PAYLOAD_TYPE(payload.Payload_PAYLOAD_TYPE_value["MSG"])
			p := &payload.Payload{
				Type: &ctype,
				Body: []byte("hello world"),
			}
			_, err := proto.Marshal(p)
			g.Assert(err == nil).Equal(true)
		})


	})
}
func TestCommunication(t *testing.T) {
	g := Goblin(t) 
	g.Describe("Communication", func() {
		g.It("Can communicate with each other", func() {
			c1ctx, c1cancel := context.WithCancel(context.Background())
			c1, err := New(c1ctx, "/tmp/.ipfs_test_1")
			g.Assert(err).Equal(nil)
			g.Assert(c1 != nil).Equal(true)
			defer c1cancel()
			defer c1.Close()

			c2ctx, c2cancel := context.WithCancel(context.Background())
			c2, err := New(c2ctx, "/tmp/.ipfs_test_2")
			g.Assert(err).Equal(nil)
			g.Assert(c2 != nil).Equal(true)
			defer c2cancel()
			defer c2.Close()

			err = c1.AddContact(c2.Node.Identity.Pretty())
			g.Assert(err).Equal(nil)

			err = c2.AddContact(c1.Node.Identity.Pretty())
			g.Assert(err).Equal(nil)

			// wait for clients to be online
			for {
				if c1.Contacts[0].IsOnline() == true {
					break
				}
				time.Sleep(time.Second * 1)
			}

			for {
				if c2.Contacts[0].IsOnline() == true {
					break
				}
				time.Sleep(time.Second * 1)
			}

			g.Assert(c1.Contacts[0].IsOnline()).Equal(true)
			g.Assert(c2.Contacts[0].IsOnline()).Equal(true)

			c1msg := c1.Contacts[0].Read()
			c2msg := c2.Contacts[0].Read()

			// wait for messages
			got1 := false
			got2 := false

			for {
				time.Sleep(time.Second * 1)

				if got1 && got2 {
					break
				}

				err = c1.Contacts[0].WriteEncryptedPayload(*c1payload)
				g.Assert(err).Equal(nil)

				err = c2.Contacts[0].WriteEncryptedPayload(*c2payload)
				g.Assert(err).Equal(nil)

				select {
				case c1got := <-c1msg:
						g.Assert(string(c1got.GetData())).Equal("hello from c2")
						got1 = true
				case c2got := <-c2msg:
						g.Assert(string(c2got.GetData())).Equal("hello from c1")
						got2 = true
				default:
					time.Sleep(time.Second * 1)
				}
			}
		})
	})
}