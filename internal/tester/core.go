package tester

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/cusconf/cusconf/internal/node"
)

// The node tester drives mihomo (https://github.com/MetaCubeX/mihomo, GPL-3.0) as a
// separate program: cusconf writes a config with one SOCKS5 listener per node, each
// pinned to that node, starts the mihomo executable and connects through the
// listeners. No mihomo code is linked into cusconf.

// ErrNoMihomo is returned when the mihomo executable cannot be found.
var ErrNoMihomo = errors.New("mihomo executable not found: install mihomo or set CUSCONF_MIHOMO to its path")

// MihomoPath locates the mihomo executable: $CUSCONF_MIHOMO, "mihomo" next to
// the cusconf executable, then "mihomo" in PATH.
func MihomoPath() (string, error) {
	if p := os.Getenv("CUSCONF_MIHOMO"); p != "" {
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "mihomo")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	if p, err := exec.LookPath("mihomo"); err == nil {
		return p, nil
	}
	return "", ErrNoMihomo
}

// Core is a running mihomo process serving a set of nodes.
type Core struct {
	dir    string
	cmd    *exec.Cmd
	ports  map[*node.Node]int
	errs   map[*node.Node]error
	exited chan struct{}
	out    bytes.Buffer
	once   sync.Once
}

// StartCore starts mihomo with a listener for each node. Nodes mihomo rejects are
// left out and report their error from Proxy; err is only for failures that
// affect every node (no executable, process did not start).
func StartCore(ctx context.Context, nodes []*node.Node) (*Core, error) {
	bin, err := MihomoPath()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "cusconf-mihomo-")
	if err != nil {
		return nil, err
	}
	c := &Core{dir: dir, ports: map[*node.Node]int{}, errs: map[*node.Node]error{}, exited: make(chan struct{})}
	ok := false
	defer func() {
		if !ok {
			c.Close()
		}
	}()

	var keep []*node.Node
	var proxies []map[string]any
	for _, n := range nodes {
		m, err := ToMihomo(n)
		if err != nil {
			c.errs[n] = err
			continue
		}
		m["name"] = fmt.Sprintf("p%d", len(proxies))
		keep = append(keep, n)
		proxies = append(proxies, m)
	}
	if len(keep) == 0 {
		ok = true
		return c, nil
	}
	ports, err := freePorts(len(keep))
	if err != nil {
		return nil, err
	}
	cfg := filepath.Join(dir, "config.yaml")

	// Let mihomo validate the config; drop the proxies it rejects, one at a time.
	for {
		if err := writeConfig(cfg, proxies, ports); err != nil {
			return nil, err
		}
		out, err := exec.CommandContext(ctx, bin, "-d", dir, "-f", cfg, "-t").CombinedOutput()
		if err == nil {
			break
		}
		i, msg := badProxy(out)
		if i < 0 || i >= len(proxies) {
			return nil, fmt.Errorf("mihomo rejected the test config: %s", lastLine(out))
		}
		c.errs[keep[i]] = fmt.Errorf("mihomo: %s", msg)
		keep = append(keep[:i:i], keep[i+1:]...)
		proxies = append(proxies[:i:i], proxies[i+1:]...)
		ports = append(ports[:i:i], ports[i+1:]...)
		if len(keep) == 0 {
			ok = true
			return c, nil
		}
		for j := range proxies { // listener names refer to proxies by name
			proxies[j]["name"] = fmt.Sprintf("p%d", j)
		}
	}

	c.cmd = exec.Command(bin, "-d", dir, "-f", cfg)
	c.cmd.Stdout, c.cmd.Stderr = &c.out, &c.out
	if err := c.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mihomo: %w", err)
	}
	go func() { c.cmd.Wait(); close(c.exited) }()
	for i, n := range keep {
		c.ports[n] = ports[i]
	}
	if err := c.waitReady(ctx, ports); err != nil {
		return nil, err
	}
	ok = true
	return c, nil
}

// Proxy returns the dialable proxy for a node passed to StartCore.
func (c *Core) Proxy(n *node.Node) (*Proxy, error) {
	if err, bad := c.errs[n]; bad {
		return nil, err
	}
	port, ok := c.ports[n]
	if !ok {
		return nil, fmt.Errorf("node not loaded into mihomo")
	}
	select {
	case <-c.exited:
		return nil, fmt.Errorf("mihomo exited: %s", lastLine(c.out.Bytes()))
	default:
	}
	return &Proxy{addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}, nil
}

// Close stops the process and removes its working directory.
func (c *Core) Close() {
	c.once.Do(func() {
		if c.cmd != nil && c.cmd.Process != nil {
			c.cmd.Process.Kill()
			<-c.exited
		}
		os.RemoveAll(c.dir)
	})
}

func (c *Core) waitReady(ctx context.Context, ports []int) error {
	deadline := time.Now().Add(20 * time.Second)
	for _, p := range ports {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(p))
		for {
			conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
			if err == nil {
				conn.Close()
				break
			}
			select {
			case <-c.exited:
				return fmt.Errorf("mihomo exited: %s", lastLine(c.out.Bytes()))
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("mihomo did not open its listeners in time")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	return nil
}

func writeConfig(path string, proxies []map[string]any, ports []int) error {
	var listeners []map[string]any
	for i, p := range proxies {
		listeners = append(listeners, map[string]any{
			"name": fmt.Sprintf("in%d", i), "type": "socks", "listen": "127.0.0.1", "port": ports[i], "udp": false, "proxy": p["name"],
		})
	}
	cfg := map[string]any{
		"mode":            "rule",
		"log-level":       "warning",
		"ipv6":            false, // IPv6 lookups of node servers stall connects when the host has no IPv6 route
		"allow-lan":       false,
		"geo-auto-update": false,
		"proxies":         proxies,
		"listeners":       listeners,
		"rules":           []string{"MATCH,DIRECT"},
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

var proxyErrRe = regexp.MustCompile(`proxy (\d+): ([^\n"]+)`)

// badProxy finds "proxy N: message" in mihomo's config test output.
func badProxy(out []byte) (int, string) {
	m := proxyErrRe.FindSubmatch(out)
	if m == nil {
		return -1, ""
	}
	i, _ := strconv.Atoi(string(m[1]))
	return i, string(bytes.TrimSpace(m[2]))
}

func lastLine(b []byte) string {
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	if len(lines) == 0 {
		return ""
	}
	return string(lines[len(lines)-1])
}

// freePorts reserves n distinct free TCP ports on 127.0.0.1.
func freePorts(n int) ([]int, error) {
	var ls []net.Listener
	defer func() {
		for _, l := range ls {
			l.Close()
		}
	}()
	ports := make([]int, 0, n)
	for range n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		ls = append(ls, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	return ports, nil
}

// socks5Dial connects to addr through the SOCKS5 server at proxy (no auth),
// passing host names through so they resolve on the node's side.
func socks5Dial(ctx context.Context, proxy, addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("bad port in %q", addr)
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", proxy)
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}
	fail := func(err error) (net.Conn, error) { conn.Close(); return nil, err }
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return fail(err)
	}
	var b [4]byte
	if _, err := io.ReadFull(conn, b[:2]); err != nil {
		return fail(err)
	}
	if b[0] != 5 || b[1] != 0 {
		return fail(errors.New("socks5: authentication rejected"))
	}
	req := []byte{5, 1, 0}
	if ip, err := netip.ParseAddr(host); err == nil && ip.Is4() {
		a := ip.As4()
		req = append(append(req, 1), a[:]...)
	} else if err == nil {
		a := ip.As16()
		req = append(append(req, 4), a[:]...)
	} else {
		if len(host) > 255 {
			return fail(errors.New("socks5: host name too long"))
		}
		req = append(append(req, 3, byte(len(host))), host...)
	}
	req = append(req, byte(port>>8), byte(port))
	if _, err := conn.Write(req); err != nil {
		return fail(err)
	}
	if _, err := io.ReadFull(conn, b[:4]); err != nil {
		return fail(err)
	}
	if b[1] != 0 {
		return fail(fmt.Errorf("socks5: connect failed (code %d)", b[1]))
	}
	var skip int
	switch b[3] {
	case 1:
		skip = 4
	case 4:
		skip = 16
	case 3:
		var l [1]byte
		if _, err := io.ReadFull(conn, l[:]); err != nil {
			return fail(err)
		}
		skip = int(l[0])
	default:
		return fail(errors.New("socks5: bad reply"))
	}
	if _, err := io.ReadFull(conn, make([]byte, skip+2)); err != nil {
		return fail(err)
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}
