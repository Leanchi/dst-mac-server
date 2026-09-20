package game

import (
	"errors"
	"testing"
	"time"
)

func TestWaitForProcessExit(t *testing.T) {
	t.Run("首次轮询即已退出", func(t *testing.T) {
		if !waitForProcessExit(func() (bool, error) { return false, nil }, time.Minute, time.Millisecond) {
			t.Fatal("进程已退出时应返回 true")
		}
	})

	t.Run("轮询数次后退出", func(t *testing.T) {
		calls := 0
		status := func() (bool, error) {
			calls++
			return calls < 3, nil // 第 3 次轮询时报告已退出
		}
		if !waitForProcessExit(status, time.Minute, time.Millisecond) {
			t.Fatal("进程在超时前退出时应返回 true")
		}
	})

	t.Run("超时仍未退出", func(t *testing.T) {
		if waitForProcessExit(func() (bool, error) { return true, nil }, 10*time.Millisecond, time.Millisecond) {
			t.Fatal("超时未退出时应返回 false")
		}
	})

	t.Run("status持续报错按未退出处理", func(t *testing.T) {
		if waitForProcessExit(func() (bool, error) { return false, errors.New("boom") }, 10*time.Millisecond, time.Millisecond) {
			t.Fatal("status 持续报错时应等到超时并返回 false")
		}
	})
}
