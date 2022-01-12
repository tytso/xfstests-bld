module thunk.org/gce-server/ltm

replace thunk.org/gce-server/util => ../util

go 1.14

require (
	cloud.google.com/go/compute v1.0.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/sirupsen/logrus v1.8.1
	golang.org/x/sys v0.0.0-20220111092808-5a964db01320 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/api v0.65.0
	google.golang.org/genproto v0.0.0-20220111164026-67b88f271998 // indirect
	google.golang.org/grpc v1.43.0 // indirect
	thunk.org/gce-server/util v0.0.0-00010101000000-000000000000
)
