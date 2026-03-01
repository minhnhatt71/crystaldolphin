package bus

import (
	"strings"
)

func RoutingKey(channel Channel, chatId string) string {
	if chatId == "" {
		return string(channel)
	}

	return string(channel) + ":" + chatId
}

// ParseRoutingKey splits a routing key into channel and chat ID.
func ParseRoutingKey(key string) (channel Channel, chatId string) {
	if i := strings.Index(key, ":"); i >= 0 {
		return Channel(key[:i]), key[i+1:]
	}

	return Channel(key), ""
}
