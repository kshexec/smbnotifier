module smbnotifier

go 1.18

replace gopkg.in/fsnotify.v1 => github.com/fsnotify/fsnotify v1.4.9

require (
	github.com/eclipse/paho.mqtt.golang v1.5.0
	github.com/farmergreg/rfsnotify v0.0.0-20240825142021-55bd5f2910f6
	gopkg.in/fsnotify.v1 v1.0.0-00010101000000-000000000000
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/net v0.27.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.22.0 // indirect
)
