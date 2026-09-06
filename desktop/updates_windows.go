//go:build windows

package desktop

import (
	"context"
	"errors"
	"os"
	"runtime"

	"github.com/Miku0139oao/rta-sales-client-go/internal/portableupdate"
)

type windowsUpdateInstaller struct{}

func nativeUpdateInstaller(version string) (updateInstaller, error) {
	if runtime.GOARCH != "amd64" {
		return nil, errors.New("only Windows amd64 portable builds support installation / 僅支援 Windows 64 位元免安裝版")
	}
	if _, err := portableupdate.ParseVersion(version); err != nil {
		return nil, errors.New("development/unknown builds cannot install updates / 開發或未知版本不能安裝更新")
	}
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	identity, err := (portableupdate.WindowsIdentityVerifier{}).VerifyIdentity(path)
	if err != nil {
		return nil, err
	}
	if !identity.Trusted || identity.PublisherSubject == "" || identity.Version != version {
		return nil, errors.New("current executable trust/version does not match this build / 目前執行檔信任或版本不符")
	}
	return windowsUpdateInstaller{}, nil
}
func (windowsUpdateInstaller) Prepare(ctx context.Context, c portableupdate.Candidate, progress func(string)) (updateReceipt, error) {
	progress("verifying-current")
	staging, err := portableupdate.NewWindowsStaging()
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			staging.Close()
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	progress("downloading")
	if err = staging.Download(ctx, portableupdate.NewClient(), c); err != nil {
		return nil, err
	}
	progress("starting-helper")
	helper, err := staging.StartHelper(ctx)
	if err != nil {
		return nil, err
	}
	success = true
	progress("ready")
	return &windowsUpdateReceipt{staging: staging, helper: helper}, nil
}

type windowsUpdateReceipt struct {
	staging *portableupdate.WindowsStaging
	helper  *portableupdate.HelperSession
}

func (r *windowsUpdateReceipt) Commit() error { return r.helper.Commit() }
func (r *windowsUpdateReceipt) Cancel() error { return r.helper.Cancel() }
func (r *windowsUpdateReceipt) Close()        { r.staging.Close() }
