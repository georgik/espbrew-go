package cluster

import (
	"context"
	"github.com/georgik/espbrew-go/pkg/protocol"
)

type Node interface {
	Start(ctx context.Context) error
	Stop() error
	State() *ClusterState
	ID() string
}

type ClusterState struct {
	Nodes   map[string]*protocol.NodeInfo
	Devices map[string]*protocol.DeviceInfo
	Cameras map[string]*protocol.CameraInfo
	Jobs    map[string]*protocol.JobInfo
}

func NewClusterState() *ClusterState {
	return &ClusterState{
		Nodes:   make(map[string]*protocol.NodeInfo),
		Devices: make(map[string]*protocol.DeviceInfo),
		Cameras: make(map[string]*protocol.CameraInfo),
		Jobs:    make(map[string]*protocol.JobInfo),
	}
}
