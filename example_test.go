package nls_test

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mmcshane/nls"
)

type Service struct {
	state            string
	makeRequestScope nls.Scoper
	stop             chan struct{}
	done             chan struct{}
}

func NewService(newScope nls.Scoper) *Service {
	return &Service{
		state:            "stopped",
		makeRequestScope: newScope,
		stop:             make(chan struct{}),
		done:             make(chan struct{}),
	}
}

func (svc *Service) Stop(ctx context.Context) error {
	close(svc.stop)
	svc.state = "stopped"
	select {
	case <-svc.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (svc *Service) State() string {
	return svc.state
}

// HandleRequest simulates a long-running request to this service by sleeping
// for the supplied duration.
func (svc *Service) HandleRequest(ctx context.Context, d time.Duration) error {
	reqscope := svc.makeRequestScope()
	defer reqscope.Exit(context.TODO())

	// spawn a watchdog agent but have it run more slowly than the request
	// processing so that it never fires (need predictable output for the test)
	err := spawnRequestWatchdog(ctx, reqscope, 10*d)
	if err != nil {
		return err
	}
	fmt.Printf("pretending to work by sleeping for %s\n", d)

	select {
	case <-time.After(d):
	case <-svc.stop:
	}
	// we don't have to clean up anything here as the deferred scope Exit
	// set up above will clean up the background request watchdog.
	return nil
}

func (svc *Service) ListenAndServe(ready chan<- struct{}) error {
	svc.state = "running"
	close(ready)
	<-svc.stop
	close(svc.done)
	return nil
}

func spawnRequestWatchdog(ctx context.Context, s *nls.Scope, d time.Duration) error {
	t := time.NewTicker(time.Duration(d))
	stop := make(chan struct{})
	done := make(chan struct{})
	return s.Spawn(ctx, func(context.Context) (nls.Reaper, error) {
		fmt.Println("launching request watchdog")
		go func() {
			defer close(done)
			for {
				select {
				case <-stop:
					fmt.Println("request interrupted, cleaning up watchdog")
					return
				case <-t.C:
					fmt.Println("watchdog check")
				}
			}
		}()

		return func(ctx context.Context) error {
			close(stop)
			select {
			case <-done:
			case <-ctx.Done():
			}
			return nil
		}, nil
	})
}

func Example() {
	mainscope := nls.NewScope()
	svc := mustSpawnService(context.TODO(), mainscope)

	fmt.Printf("svc.State() == %q\n", svc.State())

	go func() {
		svc.HandleRequest(context.TODO(), 10*time.Second)
	}()

	mainwait(1*time.Second, mainscope.Err())

	exitctx, _ := context.WithTimeout(context.Background(), time.Duration(3*time.Second))
	err := mainscope.Exit(exitctx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("svc.State() == %q\n", svc.State())

	// Output:
	// svc.State() == "running"
	// launching request watchdog
	// pretending to work by sleeping for 10s
	// demo exits after 1s
	// request interrupted, cleaning up watchdog
	// svc.State() == "stopped"
}

func mainwait(d time.Duration, errs <-chan error) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errs:
		fmt.Printf("unhandled error: %q", err)
	case sig := <-sigchan:
		fmt.Printf("received signal (%v)", sig)
	case <-time.After(d):
		fmt.Printf("demo exits after %v\n", d)
	}
}

func mustSpawnService(ctx context.Context, s *nls.Scope) *Service {
	svc := NewService(s.NewChildScope)
	ready := make(chan struct{}, 1)
	nls.MustSpawn(ctx, s, func(context.Context) (nls.Reaper, error) {
		go func() {
			if err := svc.ListenAndServe(ready); err != nil {
				s.Err() <- err
			}
		}()
		return svc.Stop, nil
	})
	<-ready
	return svc
}
