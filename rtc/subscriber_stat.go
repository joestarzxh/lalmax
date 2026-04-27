package rtc

import (
	"fmt"

	"github.com/pion/webrtc/v3"
)

func remoteAddrFromDTLSTransport(dtls *webrtc.DTLSTransport) string {
	if dtls == nil {
		return ""
	}
	return remoteAddrFromICETransport(dtls.ICETransport())
}

func remoteAddrFromICETransport(iceTransport *webrtc.ICETransport) string {
	if iceTransport == nil {
		return ""
	}

	pair, err := iceTransport.GetSelectedCandidatePair()
	if err != nil || pair == nil || pair.Remote == nil {
		return ""
	}

	return fmt.Sprintf("%s:%d", pair.Remote.Address, pair.Remote.Port)
}
