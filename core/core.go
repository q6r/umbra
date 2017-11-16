package core

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"time"
	"unsafe"

	"github.com/gtank/cryptopasta"
	"github.com/olebedev/emitter"
	"github.com/phayes/freeport"

	"gx/ipfs/QmQ93GLTtkiHfoydHVsXJxERzxQsNp9BaQvKMF6ZKXCQt9/go-ipfs/core"
	"gx/ipfs/QmQ93GLTtkiHfoydHVsXJxERzxQsNp9BaQvKMF6ZKXCQt9/go-ipfs/repo"
	"gx/ipfs/QmQ93GLTtkiHfoydHVsXJxERzxQsNp9BaQvKMF6ZKXCQt9/go-ipfs/repo/config"
	"gx/ipfs/QmQ93GLTtkiHfoydHVsXJxERzxQsNp9BaQvKMF6ZKXCQt9/go-ipfs/repo/fsrepo"
	peer "gx/ipfs/QmXYjuNuxVzXKJCfWasQk1RqkhVLDM9jtUKhqc2WPQmFSB/go-libp2p-peer"
	ic "gx/ipfs/QmaPbCnUMBohSGo3KnxEa2bHqyJVVeEEcwtqJAYxerieBo/go-libp2p-crypto"
)

var (
	errRepoExists = errors.New("repo already exists won't re-initialize")
)

type Core struct {
	Events     *emitter.Emitter
	Node       *core.IpfsNode
	Repo       repo.Repo
	RepoPath   string
	Contacts   []*Contact
	PrivateKey *rsa.PrivateKey
}

func New(ctx context.Context, path string) (*Core, error) {
	var err error

	c := &Core{}
	c.RepoPath, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	c.Events = emitter.New(1024)

	// Initialize repo
	err = c.initRepo()
	if err == errRepoExists {
		c.Repo, err = fsrepo.Open(c.RepoPath)
		if err != nil {
			return nil, errors.New("unable to open repo")
		}
	}
	if err != nil {
		return nil, errors.New("Unable to initialize repo")
	}

	// Setup node
	err = c.setupNode(ctx)
	if err != nil {
		c.Repo.Close()
		c.Repo = nil
		return nil, err
	}

	// Extract my Private Key in the good format
	c.PrivateKey, err = c.extractSelfRSAPrivateKey()
	if err != nil {
		return nil, err
	}

	go c.contactStatus()

	return c, nil
}

// contactStatus emit event on the status of contacts
func (c *Core) contactStatus() {
	for {
		time.Sleep(4 * time.Second)
		for _, contact := range c.Contacts {
			if contact.IsOnline() == true {
				c.Events.Emit("contact:online", contact)
			} else {
				c.Events.Emit("contact:offline", contact)
			}
		}
	}
}

func (c *Core) Decrypt(encryptedAesKey []byte, cipherData []byte) ([]byte, error) {

	// Decrypt the aeskey
	aeskey, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, c.PrivateKey, encryptedAesKey, []byte{})
	if err != nil {
		return []byte{}, err
	}

	// AESDecrypt the body using the decrypted aes key
	var dkey [32]byte
	copy(dkey[:], aeskey)
	plaintext, err := cryptopasta.Decrypt(cipherData, &dkey)
	if err != nil {
		return []byte{}, err
	}

	return plaintext, nil
}

// GetPeerRSAKey We decode peer id and get it's publickey from the peer
// store
func (c *Core) GetPeerPublicRSAKey(ctx context.Context, idstr string) (*rsa.PublicKey, error) {

	id, err := peer.IDB58Decode(idstr)
	if err != nil {
		return nil, err
	}

	pk := c.Node.Peerstore.PubKey(id)
	if pk == nil {
		return nil, errors.New("public key request not found in peerstore")
	}

	return extractRSAPublicKey(pk)
}

// extractRSAPublicKey given a ic.PubKey we convert it to the needed
// format *rsa.PublicKey for easier encryption...
func extractRSAPublicKey(pubkey ic.PubKey) (*rsa.PublicKey, error) {
	rs := reflect.ValueOf(pubkey).Elem()
	k := rs.FieldByName("k")
	k = reflect.NewAt(k.Type(), unsafe.Pointer(k.UnsafeAddr())).Elem()

	pub, ok := k.Interface().(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("Unable to cast publickey")
	}

	return pub, nil
}

// extractMyRSAPrivateKey extracts my key in the correct format
func (c *Core) extractSelfRSAPrivateKey() (*rsa.PrivateKey, error) {
	privk, err := c.Node.GetKey("self")
	if err != nil {
		return nil, err
	}

	rs := reflect.ValueOf(privk).Elem()

	sk := rs.FieldByName("sk")
	sk = reflect.NewAt(sk.Type(), unsafe.Pointer(sk.UnsafeAddr())).Elem()

	priv, ok := sk.Interface().(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("unable to cast")
	}

	return priv, nil
}

// Save the state of core
// inside of ipfs repository
func (c *Core) Save() error {

	// Marshal contacts
	bcontacts, err := json.Marshal(c.Contacts)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/state", c.RepoPath), bcontacts, 0755)
	if err != nil {
		return err
	}

	return nil
}

// Load the state of core
// TODO : contacts are reloaded without name, must add their name too...
func (c *Core) Load() error {

	bcontacts, err := ioutil.ReadFile(fmt.Sprintf("%s/state", c.RepoPath))
	if err != nil {
		return err
	}

	contacts := []Contact{}
	err = json.Unmarshal(bcontacts, &contacts)
	if err != nil {
		return err
	}

	// TODO : handle errors
	for _, con := range contacts {
		err := c.AddContact(con.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

// AddContact to core
func (c *Core) AddContact(id string) error {

	// Duplicates not allowed
	for _, contact := range c.Contacts {
		if contact.ID == id {
			return errors.New("id already exists")
		}
	}

	contact, err := NewContact(c, id)
	if err != nil {
		return err
	}

	c.Contacts = append(c.Contacts, contact)

	c.Events.Emit("contact:add", contact)

	return nil
}

// DeleteContact remove contact from core
func (c *Core) DeleteContact(id string) error {
	var found_contact *Contact = nil
	found_index := -1
	for index, contact := range c.Contacts {
		if contact.ID == id {
			found_index = index
			found_contact = contact
			break
		}
	}

	if found_index == -1 || found_contact == nil {
		return errors.New("unable to delete, id doesn't exist")
	}

	c.Contacts = append(c.Contacts[:found_index], c.Contacts[found_index+1:]...)
	found_contact.Close()

	c.Events.Emit("contact:delete", found_contact)

	return nil
}

// Initialize the repo
func (c *Core) initRepo() error {

	if c.RepoPath == "" {
		return errors.New("empty repository provided")
	}

	if fsrepo.IsInitialized(c.RepoPath) {
		return errRepoExists
	}

	conf, err := config.Init(os.Stdout, 2048)
	if err != nil {
		return err
	}

	if err := fsrepo.Init(c.RepoPath, conf); err != nil {
		return err
	}

	c.Repo, err = fsrepo.Open(c.RepoPath)
	if err != nil {
		return err
	}

	return nil
}

func (c *Core) setupNode(ctx context.Context) error {
	var err error

	cfg := new(core.BuildCfg)
	cfg.Repo = c.Repo
	cfg.Online = true
	cfg.Permament = true
	cfg.ExtraOpts = map[string]bool{
		"pubsub": true,
	}

	SwarmPort, err := freeport.GetFreePort()
	if err != nil {
		return err
	}
	err = c.Repo.SetConfigKey("Addresses.Swarm", []string{fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", SwarmPort),
		fmt.Sprintf("/ip6/::/tcp/%d", SwarmPort)})
	if err != nil {
		return err
	}

	APIPort, err := freeport.GetFreePort()
	if err != nil {
		return err
	}
	err = c.Repo.SetConfigKey("Addresses.API", fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", APIPort))
	if err != nil {
		return err
	}

	GatewayPort, err := freeport.GetFreePort()
	if err != nil {
		return err
	}
	err = c.Repo.SetConfigKey("Addresses.Gateway", fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", GatewayPort))
	if err != nil {
		return err
	}

	c.Node, err = core.NewNode(ctx, cfg)
	if err != nil {
		return err
	}

	return nil
}

// Close all allocated resource
func (c *Core) Close() error {

	// unsubscribe all pubsubs
	for _, contact := range c.Contacts {
		contact.Close()
	}

	// TODO : handle errors
	if err := c.Node.Close(); err != nil {
		return err
	}

	if err := c.Repo.Close(); err != nil {
		return err
	}

	return nil
}
