// Package pty 实现 PTY master 端劫持：在 slave 端 spawn 子进程（mock-opencode），
// master 端读取输出并按行解析待执行命令交给 guard 审查。
package pty

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// Supervisor 在 PTY 下运行子进程，并对输出做按行扫描。
// v0.1 采用"输出扫描"模型识别命令（mock-opencode 的事件流里包含 command 字段）；
// v0.2 接入真实 opencode 后改用 opencode 的 tool.call 事件流精确获取命令。
type Supervisor struct {
	cmd      *exec.Cmd
	tty      *os.File
	mu       sync.Mutex
	stopped  chan struct{}
	stopOnce sync.Once
	onLine   func(line string)
}

// New 创建 Supervisor。child 为待 spawn 的可执行文件路径，args 为参数。
// onLine 回调对每一行输出调用（用于 guard 解析命令）。
func New(child string, args []string, onLine func(line string)) *Supervisor {
	c := exec.Command(child, args...)
	return &Supervisor{
		cmd:     c,
		onLine:  onLine,
		stopped: make(chan struct{}),
	}
}

// Start 在 PTY 下启动子进程，阻塞直到子进程结束或 ctx 取消。
func (s *Supervisor) Start(ctx context.Context) error {
	tty, err := pty.Start(s.cmd)
	if err != nil {
		return err
	}
	s.tty = tty
	defer tty.Close()

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(tty)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if s.onLine != nil {
				s.onLine(line)
			}
		}
	}()

	// 等待退出或上下文取消
	waitCh := make(chan error, 1)
	go func() { waitCh <- s.cmd.Wait() }()
	select {
	case err := <-waitCh:
		<-scanDone
		s.markStopped()
		return err
	case <-ctx.Done():
		_ = s.cmd.Process.Kill()
		<-waitCh
		s.markStopped()
		return ctx.Err()
	}
}

// Write sends a policy decision to the controlled child over its PTY.
func (s *Supervisor) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tty == nil {
		return 0, io.ErrClosedPipe
	}
	return s.tty.Write(p)
}

// Stopped 返回关闭信号 channel。
func (s *Supervisor) Stopped() <-chan struct{} { return s.stopped }

func (s *Supervisor) markStopped() {
	s.stopOnce.Do(func() { close(s.stopped) })
}
