// SMBNotifier - Simple go program to recursively watch for changes to library
// folders for Plex and push any events to MQTT

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/farmergreg/rfsnotify"
	"gopkg.in/fsnotify.v1"
)

var libraries []string
var libWatchDirs map[string]string

type ConfigMQTT struct {
	IP       string `json:"ip"`
	Port     uint64 `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Topic    string `json:"topic"`
}

type Config struct {
	Mqtt      ConfigMQTT          `json:"mqtt"`
	Libraries map[string][]string `json:"libraries"`
}

type scans struct {
	Movies bool              `json:"movies"`
	Shows  bool              `json:"shows"`
	Events []*fsnotify.Event `json:"events"`
}

func LoadConfig(filePath string) (*Config, error) {
	configBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	err = json.Unmarshal(configBytes, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func addEventToPayload(scan map[string]interface{}, event *fsnotify.Event) {
	for watchDir, library := range libWatchDirs {
		if strings.Contains(event.Name, watchDir) {
			scan[library] = true
		}
	}
	scan["events"] = append(scan["events"].([]*fsnotify.Event), event)
}

func processEvents(watcher *rfsnotify.RWatcher, mqttClient mqtt.Client) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			scan := make(map[string]interface{})
			for _, lib := range libraries {
				scan[lib] = false
			}
			scan["events"] = []*fsnotify.Event{}
			addEventToPayload(scan, &event)
			log.Printf("Got event: %+v", event)
			// Wait and drain queue to prevent multiple dispatches
			time.Sleep(30 * time.Second)
		drain:
			for {
				select {
				case sEvent, ok := <-watcher.Events:
					if !ok {
						return
					}
					addEventToPayload(scan, &sEvent)
					log.Printf("Subsequent event: %+v", sEvent)
				default:
					log.Println("Event queue drained")
					break drain
				}
			}
			scanBytes, err := json.Marshal(scan)
			if err != nil {
				log.Printf("Error with JSON encoding: %s", err.Error())
			}
			log.Printf("Triggering scan of %s", scanBytes)
			if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
				panic(token.Error())
			}
			if token := mqttClient.Publish(
				"homeassistant/SMBNotifier", byte(0), false, string(scanBytes),
			); token.Wait() && token.Error() != nil {
				panic(token.Error())
			}
			log.Println("Published notification to MQTT broker")

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}

func main() {
	args := os.Args
	if len(args) == 1 {
		log.Fatalf("Please provide config file path, for example: %s /path/to/config.json", os.Args[0])
	}
	configFile := args[len(args)-1]
	if _, err := os.Stat(configFile); err != nil {
		log.Fatalf("Error reading config file %s: %s", configFile, err.Error())
	}
	// Read config
	config, err := LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Error reading config: %s", err.Error())
	}
	libWatchDirs = make(map[string]string)
	for lib, dirs := range config.Libraries {
		libraries = append(libraries, lib)
		for _, dir := range dirs {
			libWatchDirs[dir] = lib
		}
	}

	// Create recursive watcher
	watcher, err := rfsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create watcher: %s", err.Error())
	}
	defer watcher.Close()

	// Create MQTT client
	// mqtt.DEBUG = log.New(os.Stdout, "", 0)
	mqtt.ERROR = log.New(os.Stdout, "", 0)
	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:%d", config.Mqtt.IP, config.Mqtt.Port)).
		SetClientID("SMBNotifier").
		SetUsername(config.Mqtt.User).
		SetPassword(config.Mqtt.Password)
	// SetKeepAlive(2 * time.Second).
	// SetPingTimeout(1 * time.Second)
	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	defer mqttClient.Disconnect(250)

	// Start listening for events.
	go processEvents(watcher, mqttClient)

	for watchDir, library := range libWatchDirs {
		watcher.AddRecursive(watchDir)
		log.Printf("Added watch for %s library folder %s", library, watchDir)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	log.Println("Exiting.")
}
