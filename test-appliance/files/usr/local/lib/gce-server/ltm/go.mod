module thunk.org/gce-server/ltm

replace thunk.org/gce-server/util => ../util

go 1.14

require (
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/api v0.114.0
	google.golang.org/grpc v1.56.3 // indirect
	thunk.org/gce-server/util v0.0.0-00010101000000-000000000000
)
