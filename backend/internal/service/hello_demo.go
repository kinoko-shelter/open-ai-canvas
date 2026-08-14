package service

import (
	"fmt"
	"time"
)

func print_hell() {
	fmt.Println("Hello World")
}

func (s *Service) startHelloDemo() {
	s.backgroundTasks.Add(1)
	go func() {
		defer s.backgroundTasks.Done()
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		print_hell()

		for {
			select {
			case <-s.workerStop:
				return
			case <-ticker.C:
				print_hell()
			}
		}
	}()
}
