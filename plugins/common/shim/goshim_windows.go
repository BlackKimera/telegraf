// +build windows

package shim

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func (s *Shim) listenForCollectMetricsSignals(ctx context.Context) {
	collectMetricsPrompt := make(chan os.Signal, 1)

	signal.Notify(collectMetricsPrompt, syscall.SIGHUP)

	go func() {
		for {
			select {
			case <-ctx.Done():
				// context done. stop to signals to avoid pushing messages to a closed channel
				signal.Stop(collectMetricsPrompt)
				return
			case <-collectMetricsPrompt:
				s.pushCollectMetricsRequest()
			}
		}
	}()
}
