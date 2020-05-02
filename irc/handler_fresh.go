package irc

// freshHandler is a handler for a brand new connection that has not been
// registered yet.
type freshHandler struct {
	state chan state
}

func newFreshHandler(state chan state) handler {
	return &freshHandler{state: state}
}

func (h *freshHandler) handle(conn connection, msg message) handler {
	if msg.command == cmdQuit.command {
		conn.kill()
		return nullHandler{}
	}
	if msg.command != cmdNick.command {
		return h
	}
	return h.handleNick(conn, msg)
}

func (_ *freshHandler) closed(c connection) {
	c.kill()
}

func (h *freshHandler) handleNick(conn connection, msg message) handler {
	state := <-h.state
	defer func() { h.state <- state }()

	if len(msg.params) < 1 {
		sendNumeric(state, conn, errorNoNicknameGiven)
		return h
	}
	nick := msg.params[0]

	user := state.newUser(nick)
	if user == nil {
		sendNumeric(state, conn, errorNicknameInUse)
		return h
	}
	user.sink = conn

	return &freshUserHandler{state: h.state, user: user}
}

// freshUserHandler is a handler for a brand new connection that is in the
// process of registering and has successfully set a nickname.
type freshUserHandler struct {
	user  *user
	state chan state
}

func (h *freshUserHandler) handle(conn connection, msg message) handler {
	if msg.command == cmdQuit.command {
		state := <-h.state
		state.removeUser(h.user)
		h.state <- state
		conn.kill()
		return nullHandler{}
	}
	if msg.command != cmdUser.command {
		return h
	}
	return h.handleUser(conn, msg)
}

func (h *freshUserHandler) closed(c connection) {
	state := <-h.state
	defer func() { h.state <- state }()

	state.removeUser(h.user)
	c.kill()
}

func (h *freshUserHandler) handleUser(conn connection, msg message) handler {
	state := <-h.state
	defer func() { h.state <- state }()

	var trailing = msg.laxTrailing(3)
	if len(msg.params) < 3 || trailing == "" {
		sendNumeric(state, h.user, errorNeedMoreParams)
		return h
	}
	logf(debug, "handleUser %+v", msg)

	h.user.user = msg.params[0]

	sendMOTD(state, h.user)

	return newUserHandler(h.state, h.user.nick)
}
