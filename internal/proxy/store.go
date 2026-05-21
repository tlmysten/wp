package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type Store struct {
	stateDir string
	path     string
	lockPath string
}

func NewStore(stateDir string) (*Store, error) {
	dir := stateDir
	if dir == "" {
		defaultDir, err := DefaultStateDir()
		if err != nil {
			return nil, err
		}
		dir = defaultDir
	}

	path := filepath.Join(dir, "proxy-state.json")
	return &Store{
		stateDir: dir,
		path:     path,
		lockPath: path + ".lock",
	}, nil
}

func DefaultStateDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(configDir, "wp"), nil
}

func (store *Store) Load() (State, error) {
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return NewState(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("parse state: %w", err)
	}
	if state.Version == 0 {
		state.Version = stateVersion
	}
	if state.Services == nil {
		state.Services = make(map[string]Service)
	}
	return state, nil
}

func (store *Store) Update(ctx context.Context, update func(*State) error) error {
	unlock, err := store.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	state, err := store.Load()
	if err != nil {
		return err
	}
	if err := update(&state); err != nil {
		return err
	}
	return store.save(state)
}

func (store *Store) save(state State) error {
	if err := os.MkdirAll(store.stateDir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	state.Version = stateVersion
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(store.stateDir, "proxy-state-*.json")
	if err != nil {
		return fmt.Errorf("create temp state: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp state: %w", err)
	}
	if err := os.Rename(tmpPath, store.path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}

func (store *Store) lock(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(store.stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	file, err := os.OpenFile(store.lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state lock: %w", err)
	}

	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			if err := file.Truncate(0); err != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
				return nil, fmt.Errorf("truncate state lock: %w", err)
			}
			if _, err := file.Seek(0, 0); err != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
				return nil, fmt.Errorf("seek state lock: %w", err)
			}
			if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
				return nil, fmt.Errorf("write state lock: %w", err)
			}
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = file.Close()
			return nil, fmt.Errorf("lock state: %w", err)
		}

		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
