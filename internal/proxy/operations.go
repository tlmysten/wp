package proxy

import (
	"context"
	"fmt"
	"time"
)

func UpsertService(ctx context.Context, store *Store, name string, alias string, listenPort int) (Service, error) {
	if name == "" {
		return Service{}, fmt.Errorf("service name is required")
	}
	if alias == "" && listenPort <= 0 {
		return Service{}, fmt.Errorf("either --alias or --listen is required")
	}
	if alias != "" && listenPort > 0 {
		return Service{}, fmt.Errorf("only one of --alias or --listen can be set")
	}

	var saved Service
	err := store.Update(ctx, func(state *State) error {
		now := time.Now()
		service := state.Services[name]
		if service.Name == "" {
			service = Service{
				Name:      name,
				Instances: make(map[string]Instance),
				CreatedAt: now,
			}
		}
		if service.Instances == nil {
			service.Instances = make(map[string]Instance)
		}
		service.Alias = alias
		service.ListenPort = listenPort
		service.UpdatedAt = now
		state.Services[name] = service
		saved = service
		return nil
	})
	return saved, err
}

func RemoveService(ctx context.Context, store *Store, name string) error {
	if name == "" {
		return fmt.Errorf("service name is required")
	}
	return store.Update(ctx, func(state *State) error {
		if _, ok := state.Services[name]; !ok {
			return fmt.Errorf("unknown service %q", name)
		}
		delete(state.Services, name)
		return nil
	})
}

func RegisterInstance(ctx context.Context, store *Store, serviceName string, instance Instance) error {
	if serviceName == "" {
		return fmt.Errorf("service name is required")
	}
	if instance.ID == "" {
		return fmt.Errorf("instance id is required")
	}
	if instance.Port <= 0 {
		return fmt.Errorf("instance port is required")
	}
	return store.Update(ctx, func(state *State) error {
		service, ok := state.Services[serviceName]
		if !ok {
			return fmt.Errorf("unknown service %q; add it with `wp service add %s --alias <domain>` or `wp service add %s --listen <port>`", serviceName, serviceName, serviceName)
		}
		if service.Instances == nil {
			service.Instances = make(map[string]Instance)
		}
		if _, ok := service.Instances[instance.ID]; ok {
			return fmt.Errorf("instance %q is already registered for service %q", instance.ID, serviceName)
		}
		now := time.Now()
		instance.UpdatedAt = now
		if instance.StartedAt.IsZero() {
			instance.StartedAt = now
		}
		service.Instances[instance.ID] = instance
		service.UpdatedAt = now
		state.Services[serviceName] = service
		return nil
	})
}

func UnregisterInstance(ctx context.Context, store *Store, serviceName string, id string) error {
	if serviceName == "" {
		return fmt.Errorf("service name is required")
	}
	if id == "" {
		return fmt.Errorf("instance id is required")
	}
	return store.Update(ctx, func(state *State) error {
		service, ok := state.Services[serviceName]
		if !ok {
			return fmt.Errorf("unknown service %q", serviceName)
		}
		if _, ok := service.Instances[id]; !ok {
			return fmt.Errorf("unknown instance %q for service %q", id, serviceName)
		}
		delete(service.Instances, id)
		if service.ActiveID == id {
			service.ActiveID = ""
		}
		service.UpdatedAt = time.Now()
		state.Services[serviceName] = service
		return nil
	})
}

func SwitchInstance(ctx context.Context, store *Store, backend Backend, serviceName string, id string) (Service, Instance, error) {
	if serviceName == "" {
		return Service{}, Instance{}, fmt.Errorf("service name is required")
	}
	if id == "" {
		return Service{}, Instance{}, fmt.Errorf("instance id is required")
	}

	var switchedService Service
	var switchedInstance Instance
	err := store.Update(ctx, func(state *State) error {
		service, ok := state.Services[serviceName]
		if !ok {
			return fmt.Errorf("unknown service %q", serviceName)
		}
		instance, ok := service.Instances[id]
		if !ok {
			return fmt.Errorf("unknown instance %q for service %q", id, serviceName)
		}
		if service.Alias != "" {
			if backend == nil {
				return fmt.Errorf("proxy backend is required")
			}
			if err := backend.Apply(ctx, service, instance); err != nil {
				return err
			}
		}
		service.ActiveID = id
		service.UpdatedAt = time.Now()
		state.Services[serviceName] = service
		switchedService = service
		switchedInstance = instance
		return nil
	})
	return switchedService, switchedInstance, err
}

func ActiveInstance(state State, serviceName string) (Service, Instance, error) {
	service, ok := state.Services[serviceName]
	if !ok {
		return Service{}, Instance{}, fmt.Errorf("unknown service %q", serviceName)
	}
	if service.ActiveID == "" {
		return Service{}, Instance{}, fmt.Errorf("service %q has no active instance", serviceName)
	}
	instance, ok := service.Instances[service.ActiveID]
	if !ok {
		return Service{}, Instance{}, fmt.Errorf("active instance %q for service %q is not registered", service.ActiveID, serviceName)
	}
	return service, instance, nil
}
