package irc

import (
	"encoding/json"
	"os"
	"path"
	"time"
)

// state represents the state of this IRC server. State is not safe for
// concurrent access.
type state interface {
	// GetConfig returns the IRC server's configuration.
	getConfig() Config

	// forChannels iterates over all of the registered channels and passes a
	// pointer to each to the supplied callback.
	forChannels(func(*channel))

	// forUsers iterates over all of the registered users and passes a pointer to
	// each to the supplied callback.
	forUsers(func(*user))

	// getChannel returns a pointer to the channel struct corresponding to the
	// given channel name.
	getChannel(string) *channel

	// getUser returns a pointer to the user struct corresponding to the given
	// nickname.
	getUser(string) *user

	// newUser creates a new user with the given nickname and the appropriate
	// default values.
	newUser(string) *user

	// removeUser removes a user from this IRC server. In addition, it forces the
	// user to part from all channels that they are in.
	removeUser(*user)

	// newChannel creates a new channel with the given name and the appropriate
	// default values.
	newChannel(string) *channel

	// recycleChannel removes a channel if there are no more joined users.
	recycleChannel(*channel)

	// joinChannel adds a user to a channel. It does not perform any permissions
	// checking, it only updates pointers.
	joinChannel(*channel, *user)

	// partChannel removes a user from this channel. It sends a parting message to
	// all remaining members of the channel, and removes the channel if there are
	// no remaining users.
	partChannel(*channel, *user, string)

	// removeFromChannel silently removes a user from the given channel. It does
	// not send any messages to the channel or user. The channel will also be
	// reaped if there are no active users left.
	removeFromChannel(*channel, *user)

	// logChannelMessage writes an IRC message to the given channels log file (if
	// logging is enabled).
	logChannelMessage(channel *channel, nick, message string)
}

// stateImpl is a concrete implementation of the State interface.
type stateImpl struct {
	config   Config
	channels map[string]*channel
	users    map[string]*user
}

func newState(config Config) state {
	return &stateImpl{
		config:   config,
		channels: make(map[string]*channel),
		users:    make(map[string]*user),
	}
}

func isValidNick(nick string) bool {
	return len(nick) <= 9
}

func (s stateImpl) getConfig() Config {
	return s.config
}

func (s stateImpl) forChannels(callback func(*channel)) {
	for _, ch := range s.channels {
		callback(ch)
	}
}

func (s stateImpl) forUsers(callback func(*user)) {
	for _, u := range s.users {
		callback(u)
	}
}

func (s stateImpl) getChannel(name string) *channel {
	return s.channels[lowercase(name)]
}

func (s stateImpl) getUser(nick string) *user {
	return s.users[lowercase(nick)]
}

func (s *stateImpl) newUser(nick string) *user {
	nickLower := lowercase(nick)
	if s.users[nickLower] != nil {
		return nil
	}

	if !isValidNick(nickLower) {
		return nil
	}

	logf(debug, "Adding new user %s", nick)

	u := &user{
		nick:     nick,
		channels: make(map[*channel]bool),
	}
	s.users[nickLower] = u
	return u
}

func (s *stateImpl) removeUser(user *user) {
	logf(debug, "Removing user %s", user.nick)

	user.forChannels(func(ch *channel) {
		s.partChannel(ch, user, "QUITing")
	})

	nickLower := lowercase(user.nick)
	delete(s.users, nickLower)
}

func (s *stateImpl) newChannel(name string) *channel {
	name = lowercase(name)
	if s.channels[name] != nil {
		return nil
	}

	if name[0] != '#' && name[0] != '&' {
		return nil
	}

	ch := &channel{
		name:  name,
		users: make(map[*user]bool),
	}
	s.channels[name] = ch
	return ch
}

func (s *stateImpl) recycleChannel(channel *channel) {
	logf(debug, "Recycling channel %+v", channel)

	if channel == nil || len(channel.users) != 0 {
		return
	}
	delete(s.channels, channel.name)
}

func (s *stateImpl) joinChannel(channel *channel, user *user) {
	// Don't add a user to a channel more than once.
	if channel.users[user] {
		return
	}

	logf(debug, "Adding %+v to %+v", user, channel)

	channel.users[user] = true
	user.channels[channel] = true
}

func (s *stateImpl) partChannel(channel *channel, user *user, reason string) {
	s.removeFromChannel(channel, user)
}

func (s *stateImpl) removeFromChannel(channel *channel, user *user) {
	logf(debug, "Removing %+v from %+v", user, channel)

	delete(user.channels, channel)

	if !channel.users[user] {
		return
	}

	delete(channel.users, user)

	s.recycleChannel(channel)
}

type channelMessage struct {
	Nick      string `json:"nick"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp,string"`
}

func (s *stateImpl) logChannelMessage(channel *channel, nick, message string) {
	if !s.getConfig().Logs.LogChannelMessages {
		return
	}

	logPath := path.Join(s.getConfig().Logs.Path, channel.name)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logf(warn, "Unable to open channel log file (%s) for writing: %v", logPath, err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			logf(warn, "Unable to close channel log file (%s): %v ", logPath, err)
		}
	}()

	msg := channelMessage{
		Nick:      nick,
		Message:   message,
		Timestamp: time.Now().Unix(),
	}

	msgJson, err := json.Marshal(msg)
	if err != nil {
		logf(warn, "Unable to write log line to channel log file (%s): %v ", logPath, err)
	}
	if _, err := f.WriteString(string(msgJson) + "\n"); err != nil {
		logf(warn, "Unable to write log line to channel log file (%s): %v ", logPath, err)
	}
}
