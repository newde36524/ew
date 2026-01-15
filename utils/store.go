package utils

import "sync"

type Store[T any] struct {
	data     T
	rwLocker sync.RWMutex
	isStore  bool
}

func (s *Store[T]) Set(t T) {
	s.rwLocker.Lock()
	defer s.rwLocker.Unlock()
	s.data = t
}

func (s *Store[T]) Get() T {
	s.rwLocker.RLock()
	defer s.rwLocker.RUnlock()
	return s.data
}

func (s *Store[T]) GetOrStore(fn func() (T, error)) (T, error) {
	if !s.isStore {
		s.rwLocker.Lock()
		defer s.rwLocker.Unlock()
		if !s.isStore {
			v, err := fn()
			if err != nil {
				return v, err
			}
			s.data = v
			s.isStore = true
			return s.data, nil
		}
	}
	s.rwLocker.RLock()
	defer s.rwLocker.RUnlock()
	return s.data, nil
}
