package httptrust

import (
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/gorilla/websocket"
	"golang.org/x/sys/windows"
)

func init() {
	version := windows.RtlGetVersion()
	if version.MajorVersion != 6 || version.MinorVersion != 1 {
		return
	}
	if err := install(); err != nil {
		// Leave native certificate verification intact if the local stores cannot
		// be read. Offline functionality remains available.
		log.Printf("initialize Win7 HTTPS roots: %v", err)
	}
}

func install() error {
	var system []*x509.Certificate
	blocked := blockedCertificates{}
	for _, scope := range []uint32{windows.CERT_SYSTEM_STORE_CURRENT_USER, windows.CERT_SYSTEM_STORE_LOCAL_MACHINE} {
		if err := readStore(scope, "Disallowed", func(der []byte) {
			blocked[sha256.Sum256(der)] = true
		}); err != nil {
			return err
		}
		if err := readStore(scope, "ROOT", func(der []byte) {
			if cert, err := x509.ParseCertificate(der); err == nil {
				system = append(system, cert)
			}
		}); err != nil {
			return err
		}
	}
	roots, err := rootPool(system, blocked)
	if err != nil {
		return err
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return errors.New("default HTTP transport cannot be configured")
	}
	transport := base.Clone()
	transport.TLSClientConfig = configureTLS(base.TLSClientConfig, roots, blocked)
	http.DefaultTransport = transport
	// The Feishu SDK obtains its gateway URL over HTTP, then connects through
	// Gorilla's default dialer. Both stages need the same verified roots.
	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = configureTLS(dialer.TLSClientConfig, roots, blocked)
	websocket.DefaultDialer = &dialer
	return nil
}

func readStore(scope uint32, name string, visit func([]byte)) error {
	storeName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	flags := scope | windows.CERT_STORE_OPEN_EXISTING_FLAG | windows.CERT_STORE_READONLY_FLAG
	store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM_W, 0, 0, flags, uintptr(unsafe.Pointer(storeName)))
	runtime.KeepAlive(storeName)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.Errno(windows.CRYPT_E_NOT_FOUND)) {
			return nil
		}
		return fmt.Errorf("open Windows %s certificate store: %w", name, err)
	}
	defer windows.CertCloseStore(store, 0)
	var cert *windows.CertContext
	for {
		cert, err = windows.CertEnumCertificatesInStore(store, cert)
		if err != nil {
			if errors.Is(err, syscall.Errno(windows.CRYPT_E_NOT_FOUND)) {
				return nil
			}
			return fmt.Errorf("read Windows %s certificate store: %w", name, err)
		}
		// Enumeration frees the previous context. Parsed certificates must own
		// their bytes rather than retaining a view into the Windows store.
		der := append([]byte(nil), unsafe.Slice(cert.EncodedCert, cert.Length)...)
		visit(der)
	}
}
