package logic

// SubscriberStat is the runtime traffic snapshot for a lalmax external subscriber.
type SubscriberStat struct {
	RemoteAddr    string
	ReadBytesSum  uint64
	WroteBytesSum uint64
}

// SubscriberStatProvider exposes runtime traffic stats for ext_subs sessions.
type SubscriberStatProvider interface {
	GetSubscriberStat() SubscriberStat
}
