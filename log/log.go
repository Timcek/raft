package log

type Message struct {
	Term     int
	Index    int
	Msg      interface{}
	Commited bool
}
