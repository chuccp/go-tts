package tts

import (
	"fmt"
	"time"
)

type EdgeDirectConnection struct {
	URL string `json:"url"`
}

func NewEdgeDirectConnection(now time.Time) EdgeDirectConnection {
	connectionID := randomHex(16)
	return EdgeDirectConnection{
		URL: fmt.Sprintf("%s?TrustedClientToken=%s&ConnectionId=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s",
			edgeSpeechEndpoint,
			edgeClientToken,
			connectionID,
			generateEdgeToken(now),
			edgeTokenVersion,
		),
	}
}
