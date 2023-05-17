package graceful

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type GracefulShutdown struct {
	wg   sync.WaitGroup
	once sync.Once
}

var shutdownInstance *GracefulShutdown
var mu sync.Mutex

// Singleton
func Shutdown() *GracefulShutdown {
	mu.Lock()
	defer mu.Unlock()
	if shutdownInstance == nil {
		shutdownInstance = &GracefulShutdown{}
		shutdownInstance.init()
	}

	return shutdownInstance
}
func (gs *GracefulShutdown) init() {
	gs.once.Do(func() {
		gs.wg = sync.WaitGroup{}

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			// Block until a signal is received.
			sig := <-sigChan

			// Block until all tasks are done.
			gs.Wait()

			// Exit with the signal status.
			os.Exit(int(sig.(syscall.Signal)))
		}()
	})
}

func (gs *GracefulShutdown) AddTask() {
	gs.wg.Add(1)
}

func (gs *GracefulShutdown) DoneTask() {
	gs.wg.Done()
}

func (gs *GracefulShutdown) Wait() {
	gs.wg.Wait()
	fmt.Println("Graceful shutdown complete.")
}
