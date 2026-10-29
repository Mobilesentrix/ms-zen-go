package zen

/*
#cgo LDFLAGS: -pthread -lzen_ffi
#cgo darwin LDFLAGS: -framework Security -framework SystemConfiguration
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/deps/darwin_amd64
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/deps/darwin_arm64
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/deps/linux_amd64 -lgcc_s -lutil -lrt -lm -ldl -lc
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/deps/linux_arm64 -lgcc_s -lutil -lrt -lm -ldl -lc
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/deps/windows_amd64 -lws2_32 -luserenv -lntdll -lbcrypt -lcrypt32 -lsecur32 -lncrypt
*/
import "C"

import (
	_ "github.com/Mobilesentrix/ms-zen-go/v2/deps/darwin_amd64"
	_ "github.com/Mobilesentrix/ms-zen-go/v2/deps/darwin_arm64"
	_ "github.com/Mobilesentrix/ms-zen-go/v2/deps/linux_amd64"
	_ "github.com/Mobilesentrix/ms-zen-go/v2/deps/linux_arm64"
	_ "github.com/Mobilesentrix/ms-zen-go/v2/deps/windows_amd64"
)
