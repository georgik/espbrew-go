package cluster

import (
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"context"
)

type Node interface {
	Start(ctx context.Context) error
	Stop() error
	State() *ClusterState
}

type ClusterState struct {
	Nodes   map[string]*protocol.NodeInfo
	Devices map[string]*protocol.DeviceInfo
	Jobs    map[string]*protocol.JobInfo
}

func NewClusterState() *ClusterState {
	return &ClusterState{
		Nodes:   make(map[string]*protocol.NodeInfo),
		Devices: make(map[string]*protocol.DeviceInfo),
		Jobs:    make(map[string]*protocol.JobInfo),
	}
}
