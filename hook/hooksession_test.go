package hook

import (
	"sync"
	"testing"
	"time"

	"github.com/q191201771/lal/pkg/base"
)

type recordSubscriber struct {
	msgs []base.RtmpMsg
}

func (s *recordSubscriber) OnMsg(msg base.RtmpMsg) {
	s.msgs = append(s.msgs, msg.Clone())
}

func (s *recordSubscriber) OnStop() {}

type blockingSubscriber struct {
	mu        sync.Mutex
	msgs      []base.RtmpMsg
	blocked   chan struct{}
	release   chan struct{}
	replaying bool
	blockOnce sync.Once
}

func newBlockingSubscriber() *blockingSubscriber {
	return &blockingSubscriber{
		blocked: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (s *blockingSubscriber) OnMsg(msg base.RtmpMsg) {
	s.mu.Lock()
	s.msgs = append(s.msgs, msg.Clone())
	shouldBlock := s.replaying
	s.mu.Unlock()

	if shouldBlock {
		s.blockOnce.Do(func() {
			close(s.blocked)
			<-s.release
		})
	}
}

func (s *blockingSubscriber) OnStop() {}

func (s *blockingSubscriber) OnReplayStart() {
	s.mu.Lock()
	s.replaying = true
	s.mu.Unlock()
}

func (s *blockingSubscriber) OnReplayStop() {
	s.mu.Lock()
	s.replaying = false
	s.mu.Unlock()
}

func (s *blockingSubscriber) markers() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]byte, 0, len(s.msgs))
	for _, msg := range s.msgs {
		out = append(out, payloadMarker(msg))
	}
	return out
}

func videoSeqHeader(marker byte) base.RtmpMsg {
	return base.RtmpMsg{
		Header: base.RtmpHeader{MsgTypeId: base.RtmpTypeIdVideo},
		Payload: []byte{
			base.RtmpAvcKeyFrame,
			base.RtmpAvcPacketTypeSeqHeader,
			0, 0, 0,
			marker,
		},
	}
}

func videoKeyNalu(marker byte) base.RtmpMsg {
	return base.RtmpMsg{
		Header: base.RtmpHeader{MsgTypeId: base.RtmpTypeIdVideo},
		Payload: []byte{
			base.RtmpAvcKeyFrame,
			base.RtmpAvcPacketTypeNalu,
			0, 0, 0,
			marker,
		},
	}
}

func videoInterNalu(marker byte) base.RtmpMsg {
	return base.RtmpMsg{
		Header: base.RtmpHeader{MsgTypeId: base.RtmpTypeIdVideo},
		Payload: []byte{
			base.RtmpAvcInterFrame,
			base.RtmpAvcPacketTypeNalu,
			0, 0, 0,
			marker,
		},
	}
}

func aacSeqHeader(marker byte) base.RtmpMsg {
	return base.RtmpMsg{
		Header: base.RtmpHeader{MsgTypeId: base.RtmpTypeIdAudio},
		Payload: []byte{
			base.RtmpSoundFormatAac << 4,
			base.RtmpAacPacketTypeSeqHeader,
			marker,
		},
	}
}

func aacRaw(marker byte) base.RtmpMsg {
	return base.RtmpMsg{
		Header: base.RtmpHeader{MsgTypeId: base.RtmpTypeIdAudio},
		Payload: []byte{
			base.RtmpSoundFormatAac << 4,
			base.RtmpAacPacketTypeRaw,
			marker,
		},
	}
}

func g711aAudio(marker byte) base.RtmpMsg {
	return base.RtmpMsg{
		Header:  base.RtmpHeader{MsgTypeId: base.RtmpTypeIdAudio},
		Payload: []byte{base.RtmpSoundFormatG711A<<4 | marker},
	}
}

func payloadMarker(msg base.RtmpMsg) byte {
	return msg.Payload[len(msg.Payload)-1]
}

func TestAddConsumerReplaysCachedGopImmediately(t *testing.T) {
	session := NewHookSession("test-replay", "test-replay", nil, 1, 0)
	defer GetHookSessionManagerInstance().RemoveHookSession("test-replay")

	session.OnMsg(videoSeqHeader(1))
	session.OnMsg(aacSeqHeader(2))
	session.OnMsg(videoKeyNalu(3))
	session.OnMsg(aacRaw(4))
	session.OnMsg(videoInterNalu(5))

	sub := &recordSubscriber{}
	session.AddConsumer("consumer", sub)

	if len(sub.msgs) != 5 {
		t.Fatalf("expected 5 replay messages, got %d", len(sub.msgs))
	}

	wantMarkers := []byte{1, 2, 3, 4, 5}
	for i, want := range wantMarkers {
		if got := payloadMarker(sub.msgs[i]); got != want {
			t.Fatalf("message %d marker = %d, want %d", i, got, want)
		}
	}
}

func TestVideoSeqHeaderChangeClearsStaleGop(t *testing.T) {
	session := NewHookSession("test-clear", "test-clear", nil, 1, 0)
	defer GetHookSessionManagerInstance().RemoveHookSession("test-clear")

	session.OnMsg(videoSeqHeader(1))
	session.OnMsg(videoKeyNalu(2))
	session.OnMsg(videoInterNalu(3))
	session.OnMsg(videoSeqHeader(4))

	sub := &recordSubscriber{}
	session.AddConsumer("consumer", sub)
	if len(sub.msgs) != 0 {
		t.Fatalf("expected no stale GOP replay after sequence header change, got %d messages", len(sub.msgs))
	}

	session.OnMsg(videoKeyNalu(5))
	if len(sub.msgs) != 2 {
		t.Fatalf("expected new header and current key frame, got %d messages", len(sub.msgs))
	}
	if got := payloadMarker(sub.msgs[0]); got != 4 {
		t.Fatalf("header marker = %d, want 4", got)
	}
	if got := payloadMarker(sub.msgs[1]); got != 5 {
		t.Fatalf("key frame marker = %d, want 5", got)
	}
}

func TestNonAacAudioIsNotReplayedAsHeader(t *testing.T) {
	session := NewHookSession("test-g711", "test-g711", nil, 1, 0)
	defer GetHookSessionManagerInstance().RemoveHookSession("test-g711")

	session.OnMsg(videoSeqHeader(1))
	session.OnMsg(videoKeyNalu(2))
	session.OnMsg(g711aAudio(3))

	sub := &recordSubscriber{}
	session.AddConsumer("consumer", sub)

	if len(sub.msgs) != 3 {
		t.Fatalf("expected video header, key frame and one G711 packet, got %d messages", len(sub.msgs))
	}

	wantMarkers := []byte{1, 2, base.RtmpSoundFormatG711A<<4 | 3}
	for i, want := range wantMarkers {
		if got := payloadMarker(sub.msgs[i]); got != want {
			t.Fatalf("message %d marker = %d, want %d", i, got, want)
		}
	}
}

func TestAddConsumerWithReplayDisabledDoesNotReplayCachedGop(t *testing.T) {
	session := NewHookSession("test-no-replay", "test-no-replay", nil, 1, 0)
	defer GetHookSessionManagerInstance().RemoveHookSession("test-no-replay")

	session.OnMsg(videoSeqHeader(1))
	session.OnMsg(videoKeyNalu(2))
	session.OnMsg(videoInterNalu(3))

	sub := &recordSubscriber{}
	session.AddConsumerWithReplay("consumer", sub, false)

	if len(sub.msgs) != 0 {
		t.Fatalf("expected no cached messages when replay is disabled, got %d messages", len(sub.msgs))
	}

	session.OnMsg(videoInterNalu(4))
	if len(sub.msgs) != 0 {
		t.Fatalf("expected to wait for next key frame, got %d messages", len(sub.msgs))
	}

	session.OnMsg(videoKeyNalu(5))
	if len(sub.msgs) != 2 {
		t.Fatalf("expected header and current key frame, got %d messages", len(sub.msgs))
	}
	if got := payloadMarker(sub.msgs[0]); got != 1 {
		t.Fatalf("header marker = %d, want 1", got)
	}
	if got := payloadMarker(sub.msgs[1]); got != 5 {
		t.Fatalf("key frame marker = %d, want 5", got)
	}
}

func TestAddConsumerReplayDoesNotInterleaveWithLiveKeyFrame(t *testing.T) {
	session := NewHookSession("test-replay-order", "test-replay-order", nil, 1, 0)
	defer GetHookSessionManagerInstance().RemoveHookSession("test-replay-order")

	session.OnMsg(videoSeqHeader(1))
	session.OnMsg(videoKeyNalu(2))
	session.OnMsg(videoInterNalu(3))

	sub := newBlockingSubscriber()
	addDone := make(chan struct{})
	go func() {
		session.AddConsumer("consumer", sub)
		close(addDone)
	}()

	<-sub.blocked

	liveDone := make(chan struct{})
	go func() {
		session.OnMsg(videoKeyNalu(4))
		close(liveDone)
	}()

	select {
	case <-liveDone:
		t.Fatal("live key frame should not be delivered before cached GOP replay finishes")
	case <-time.After(50 * time.Millisecond):
	}

	close(sub.release)
	<-addDone
	<-liveDone

	wantMarkers := []byte{1, 2, 3, 4}
	gotMarkers := sub.markers()
	if len(gotMarkers) != len(wantMarkers) {
		t.Fatalf("markers = %v, want %v", gotMarkers, wantMarkers)
	}
	for i, want := range wantMarkers {
		if got := gotMarkers[i]; got != want {
			t.Fatalf("message %d marker = %d, want %d, all=%v", i, got, want, gotMarkers)
		}
	}
}
