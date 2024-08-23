package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"

	"github.com/gorilla/websocket"
)

var (
	addr       = flag.String("addr", "localhost:8073", "openwebrx service address")
	squelch    = flag.Int("sq", -120, "squech level")
	freqOffset = flag.Int("offset", 0, "frequency offset")
)

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := setupInterruptHandler()

	conn, done := connectToWebSocket()
	defer conn.Close()

	go handleMessages(conn, done)

	initializeConnection(conn)

  startAudio(conn)

	mainLoop(conn, interrupt, done)
}

func setupInterruptHandler() chan os.Signal {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	return interrupt
}

func connectToWebSocket() (*websocket.Conn, chan struct{}) {
	u := url.URL{Scheme: "ws", Host: *addr, Path: "/ws/"}
	log.Printf("Connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	done := make(chan struct{})
	return conn, done
}

func handleMessages(conn *websocket.Conn, done chan struct{}) {
	defer close(done)

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			return
		}

		switch messageType {
		case websocket.BinaryMessage:
			handleBinaryMessage(message)
		case websocket.TextMessage:
			handleTextMessage(message)
		default:
			log.Println("Received unknown message type")
		}
	}
}

func handleBinaryMessage(message []byte) {
	if len(message) == 0 {
		return
	}

	firstByte := message[0]
	// data := message[1:]

	switch firstByte {
	case 1:
		// Handle FFT
	case 2:
		log.Println("Audio data received")
	case 4:
		log.Println("HD audio data received")
	default:
		log.Println("Unhandled binary message type")
	}
}

func handleTextMessage(message []byte) {
	var msgData map[string]interface{}
	err := json.Unmarshal(message, &msgData)
	if err != nil {
		handleTextParsingError(message, err)
		return
	}

	if msgType, ok := msgData["type"].(string); ok && msgType == "smeter" {
		if value, ok := msgData["value"]; ok {
			log.Printf("Smeter [absolute]: %v", value)
		}
	}
}

func handleTextParsingError(message []byte, err error) {
	if strings.HasPrefix(string(message), "CLIENT DE SERVER") {
		log.Println(string(message))
	} else {
		log.Printf("Error parsing text message: %v", err)
		log.Printf("Raw message: %s", string(message))
	}
}

func initializeConnection(conn *websocket.Conn) {
	sendMessage(conn, "SERVER DE CLIENT client=openwebrx.js type=receiver")

	sendMessage(conn, map[string]interface{}{
		"params": map[string]interface{}{
			"hd_output_rate": 44100,
			"output_rate":    11025,
		},
		"type": "connectionproperties",
	})

	sendMessage(conn, map[string]interface{}{
		"params": map[string]interface{}{
			"audio_service_id": 0,
			"dmr_filter":       3,
			"high_cut":         4000,
			"low_cut":          -4000,
			"mod":              "nfm",
			"offset_freq":      *freqOffset,
			"secondary_mod":    false,
			"squelch_level":    *squelch,
		},
		"type": "dspcontrol",
	})
}

func startAudio(conn *websocket.Conn) {
	sendMessage(conn, map[string]interface{}{
		"type":   "dspcontrol",
		"action": "start",
	})
}

func sendMessage(conn *websocket.Conn, message interface{}) {
	var msg []byte
	var err error

	switch m := message.(type) {
	case string:
		msg = []byte(m)
	case map[string]interface{}:
		msg, err = json.Marshal(m)
		if err != nil {
			log.Printf("Error marshalling JSON: %v", err)
			return
		}
	}

	err = conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		log.Printf("Error sending message: %v", err)
	}
}

func mainLoop(conn *websocket.Conn, interrupt chan os.Signal, done chan struct{}) {
	for {
		select {
		case <-done:
			log.Println("Connection closed")
			return
		case <-interrupt:
			log.Println("Interrupt received, closing connection")
			closeConnection(conn, done, interrupt)
			return
		}
	}
}

func closeConnection(conn *websocket.Conn, done chan struct{}, interrupt chan os.Signal) {
	err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Printf("Error during close: %v", err)
		return
	}

	select {
	case <-done:
	case <-interrupt:
	}
}
