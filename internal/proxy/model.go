package proxy

import (
	"sort"
	"time"
)

const stateVersion = 1

type State struct {
	Version  int                `json:"version"`
	Services map[string]Service `json:"services"`
}

type Service struct {
	Name      string              `json:"name"`
	Alias     string              `json:"alias"`
	ActiveID  string              `json:"activeId,omitempty"`
	Instances map[string]Instance `json:"instances"`
	CreatedAt time.Time           `json:"createdAt"`
	UpdatedAt time.Time           `json:"updatedAt"`
}

type Instance struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	URL       string    `json:"url"`
	CWD       string    `json:"cwd"`
	Command   []string  `json:"command"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func NewState() State {
	return State{
		Version:  stateVersion,
		Services: make(map[string]Service),
	}
}

func (state State) SortedServices() []Service {
	services := make([]Service, 0, len(state.Services))
	for _, service := range state.Services {
		services = append(services, service)
	}
	sort.Slice(services, func(i int, j int) bool {
		return services[i].Name < services[j].Name
	})
	return services
}

func (service Service) SortedInstances() []Instance {
	instances := make([]Instance, 0, len(service.Instances))
	for _, instance := range service.Instances {
		instances = append(instances, instance)
	}
	sort.Slice(instances, func(i int, j int) bool {
		if instances[i].ID == instances[j].ID {
			return instances[i].Port < instances[j].Port
		}
		return instances[i].ID < instances[j].ID
	})
	return instances
}
