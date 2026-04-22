package logic

type StreamKey struct {
	// AppName 为空表示兼容历史的 streamName 单键查找。
	AppName    string
	StreamName string
}

func NewStreamKey(appName, streamName string) StreamKey {
	return StreamKey{
		AppName:    appName,
		StreamName: streamName,
	}
}

func StreamKeyFromStreamName(streamName string) StreamKey {
	return NewStreamKey("", streamName)
}

func (key StreamKey) Valid() bool {
	return key.StreamName != ""
}

func (key StreamKey) String() string {
	if key.AppName == "" {
		return key.StreamName
	}
	return key.AppName + "/" + key.StreamName
}
