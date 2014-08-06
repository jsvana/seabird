package auth

import (
	"encoding/hex"
	"crypto/md5"
	"fmt"
	"hash"
	"io"
	"strings"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"bitbucket.org/belak/irc"
	"bitbucket.org/belak/irc/mux"
)

type GenericAccount struct {
	Id    bson.ObjectId `bson:"_id"`
	Name  string        `bson:"name"`
	Perms []string      `bson:"perms,omitempty"`
}

type User struct {
	CurrentNick string
	Account string
	Channels []string
}

type GenericAuth struct {
	Client *irc.Client
	C      *mgo.Collection
	Users  map[string]*User
	Salt   string
}

func (au *GenericAuth) getHash() hash.Hash {
	h := md5.New()
	io.WriteString(h, au.Salt)
	return h
}

func (au *GenericAuth) loginHandler(c *irc.Client, e *irc.Event) {
	u := au.GetUser(e.Identity.Nick)
	if u.Account != "" {
		c.MentionReply(e, "you are already logged in")
		return
	}

	args := strings.SplitN(e.Trailing(), " ", 2)
	if len(args) != 2 {
		c.MentionReply(e, "usage: !login username password")
		return
	}

	h := au.getHash()
	io.WriteString(h, args[1])

	pw :=hex.EncodeToString(h.Sum(nil))
	fmt.Printf("%s --- %s --- %s\n", au.Salt, args[1], pw)

	cnt, err := au.C.Find(bson.M{
		"name": args[0],
		"password": pw,
	}).Count()

	if err != nil {
		fmt.Println(err)
		return
	}

	if cnt > 0 {
		u.Account = args[0]
		au.Client.MentionReply(e, "you are now logged in as %s", args[0])
	} else {
		au.Client.MentionReply(e, "login failed")
	}
}

func NewGenericAuth(c *irc.Client, db *mgo.Database, salt string) *GenericAuth{
	au := &GenericAuth{Client: c, C: db.C("generic_auth_accounts"), Salt: salt}
	au.trackUsers()

	cmds := mux.NewCommandMux("!")
	cmds.EventFunc("login", au.loginHandler)

	c.Event("PRIVMSG", cmds)

	return au
}

func (au *GenericAuth) CheckPerm(p string, h irc.Handler) irc.Handler {
	return h
}

func (au *GenericAuth) CheckPermFunc(p string, f irc.HandlerFunc) irc.HandlerFunc{
	return f
}



// user tracking utilities

func (au *GenericAuth) GetUser(nick string) *User{
	u, ok := au.Users[nick]
	if !ok {
		u = &User{CurrentNick: nick}
	}

	return u
}

func (au *GenericAuth) addChannelToNick(c, n string) {
	u := au.GetUser(n)

	for i := 0; i < len(u.Channels); i++ {
		if u.Channels[i] == c {
			return
		}
	}

	u.Channels = append(u.Channels, c)
	au.Users[n] = u
}

func (au *GenericAuth) removeChannelFromUser(c string, u *User) {
	for i := 0; i < len(u.Channels); i++ {
		if u.Channels[i] == c {
			// Swap with last element and shrink slice
			u.Channels[i] = u.Channels[len(u.Channels)-1]
			u.Channels = u.Channels[:len(u.Channels)-1]
			break
		}
	}

	if len(u.Channels) == 0 {
		// Removing user
		delete(au.Users, u.CurrentNick)
	}
}

// user tracking

func (au *GenericAuth) connectHandler(c *irc.Client, e *irc.Event) {
	au.Users = make(map[string]*User)
}

func (au *GenericAuth) joinHandler(c *irc.Client, e *irc.Event) {
	if e.Identity.Nick != c.CurrentNick() {
		au.addChannelToNick(e.Args[0], e.Identity.Nick)
	} else {
		for _, user := range au.Users {
			au.removeChannelFromUser(e.Args[0], user)
		}
	}
}

func (au *GenericAuth) nickHandler(c *irc.Client, e *irc.Event) {
	u := au.GetUser(e.Identity.Nick)
	if len(u.Channels) == 0 {
		return
	}

	u.CurrentNick = e.Args[1]
	delete(au.Users, e.Identity.Nick)
	au.Users[u.CurrentNick] = u
}

func (au *GenericAuth) partHandler(c *irc.Client, e *irc.Event) {
	if e.Identity.Nick != c.CurrentNick() {
		if u, ok := au.Users[e.Identity.Nick]; ok {
			au.removeChannelFromUser(e.Args[0], u)
		}
	} else {
		for _, u := range au.Users {
			au.removeChannelFromUser(e.Args[0], u)
		}
	}
}

func (au *GenericAuth) quitHandler(c *irc.Client, e *irc.Event) {
	// TODO implement this
}

func (au *GenericAuth) trackUsers() {
	au.Client.EventFunc("001",  au.connectHandler)
	au.Client.EventFunc("JOIN", au.joinHandler)
	au.Client.EventFunc("NICK", au.nickHandler)
	au.Client.EventFunc("PART", au.partHandler)
	au.Client.EventFunc("QUIT", au.quitHandler)
}

