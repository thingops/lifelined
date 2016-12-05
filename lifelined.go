package main;

import "net"
import "bufio"
import "strings"
import "time"
import "github.com/rcrowley/go-metrics"
import "github.com/armon/go-proxyproto"
import "log"
import "sync"

var connection_counter metrics.Counter

type Lifeline struct {
    index string;
    Properties map[string]string;
    Local bool

    cx chan []byte;
    conn net.Conn;
}

var lifelines map[string]*Lifeline
var lifelinesMutex = &sync.Mutex{}


func NewLifeline(conn net.Conn, props map[string]string) *Lifeline{
    f := new(Lifeline)
    f.Local = true
    f.Properties = props
    f.conn = conn;
    f.cx = make(chan []byte)
    f.index = conn.RemoteAddr().String();

    if (props["X-LF-Name"] != "") {
        f.index = props["X-LF-Name"]
    }

    lifelinesMutex.Lock()
    defer lifelinesMutex.Unlock();
    var deleteme, exists = lifelines[f.index]
    if exists {
        log.Println("killing dup connection", f.index);
        deleteme.Release(false);
    }
    lifelines[f.index] = f

    log.Println("lifeline", f.index, "connected from", conn.RemoteAddr().String());
	registerLifeline(f, true);
    return f
}

func (lf *Lifeline) Release(lock bool) {
    if (lf.conn != nil) {
        lf.conn.Close();
    }

    if (lock) {
        lifelinesMutex.Lock()
        defer lifelinesMutex.Unlock();
    }
    if (lifelines[lf.index] == lf) {
        lifelines[lf.index] = nil
        delete(lifelines, lf.index);
    }
    log.Println("lifeline released:", lf.index);
}

func (lf *Lifeline) Enable() {

    lf.conn.Write([]byte(
        "HTTP/1.1 101 Switching Protocols\r\n" +
        "Upgrade: websocket\r\n" +
        "Connection: Upgrade\r\n" +
        "Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n" +
        "Sec-WebSocket-Protocol: chat\r\n" +
        "I-Cant-Believe-Its-Not: butter\r\n" +
        "\r\n"))
}

func lifelineHandler(conn net.Conn) {
    defer conn.Close();
    connection_counter.Inc(1)
    defer connection_counter.Dec(1);

    tcpcon := conn.(*proxyproto.Conn)
 //   tcpcon.SetKeepAlivePeriod(30 *time.Second)
 //   tcpcon.SetKeepAlive(true)

    bio := bufio.NewScanner(conn);
    tcpcon.SetReadDeadline(time.Now().Add(10 * time.Second));
    if !bio.Scan() {
        log.Println("never got helo", conn.RemoteAddr(), string(bio.Bytes()), bio.Err());
        return;
    }
    line := strings.TrimSpace(bio.Text());

    if (line != "GET /lifeline/1 HTTP/1.1") {
        log.Println("invalid client helo", conn.RemoteAddr(), string(line))
        return;
    }


    properties := make(map[string]string)

    for {
        tcpcon.SetReadDeadline(time.Now().Add(10 * time.Second));
        if !bio.Scan() {
            log.Println("died in the middle", properties, bio.Err());
            break;
        }
        line := strings.TrimSpace(bio.Text());
        if (line == "") {
            tcpcon.SetReadDeadline(time.Time{});
            lf := NewLifeline(conn, properties)
            defer lf.Release(true)
            sock2chan(conn, lf.cx)
            break;

        } else {
            ss := strings.Split(line, ":")
            if (len(ss) < 2) {
                continue;
            }
            properties[strings.TrimSpace(ss[0])] = strings.TrimSpace(ss[1]);
        }
    }
    log.Println("end lifelineHandler");
}

func LifeLineServer() {
    connection_counter = metrics.NewCounter()
    metrics.Register("lifeline.connections", connection_counter)

    list,err  := net.Listen("tcp", Config.LFAddr())
    proxyList := &proxyproto.Listener{Listener: list}

    if err != nil {
        log.Panic("listen failed");
        return
    }
    log.Println("lifelined on", Config.LFAddr());
    for {
        conn, err := proxyList.Accept()
        if err != nil {
            log.Panic(err);
            return
        }
        go lifelineHandler(conn)
    }
}


func sock2chan(conn net.Conn, cx chan []byte) {
    defer close(cx)
    bio := bufio.NewReader(conn);
    for {
        buf  := make([]byte, 1024)
        n, _ := bio.Read(buf)
        if (n < 1){
            break;
        }
        cx <- buf[:n]
    }
}
