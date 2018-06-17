package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/davecgh/go-spew/spew"
	"golang.org/x/net/websocket"
)

var (
	host        = flag.String("host", ":8080", "http server")
	docker_host = flag.String("docker_host", "127.0.0.1:2375", "Docker host")
	cols        = flag.Int("cols", 120, "windows cols")
	rows        = flag.Int("rows", 28, "windows rows")
)

type htmlVarDict struct {
	ContainerId string
	Cols        int
	Rows        int
}

func init() {
	flag.Parse()
}

func main() {
	http.Handle("/exec", websocket.Handler(ExecContainer))
	http.HandleFunc("/", HomeHandler)
	if err := http.ListenAndServe(*host, nil); err != nil {
		glog.Fatalln("start http server failed, ", err)
		panic(err)
	}
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		html := "missing parameter <id>"
		fmt.Fprintf(w, html)
		return
	}

	dict := &htmlVarDict{
		ContainerId: id,
		Cols:        *cols,
		Rows:        *rows,
	}

	homeTemplate.Execute(w, dict)
}

func ExecContainer(ws *websocket.Conn) {
	container := ws.Request().URL.Query().Get("id")
	if container == "" {
		ws.Write([]byte("Container does not exist"))
		return
	}

	remote_host := ws.Request().URL.Query().Get("host")
	if remote_host == "" {
		remote_host = *docker_host
		if strings.Index(remote_host, ":") == -1 {
			remote_host = remote_host + ":2375"
		}
	}

	type stuff struct {
		Id string
	}
	var s stuff
	params := bytes.NewBufferString("{\"AttachStdin\":true,\"AttachStdout\":true,\"AttachStderr\":true,\"Tty\":true,\"Cmd\":[\"/bin/sh\"]}")
	resp, err := http.Post("http://"+remote_host+"/containers/"+container+"/exec", "application/json", params)
	if err != nil {
		panic(err)
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	json.Unmarshal([]byte(data), &s)
	if err := hijack(remote_host, "POST", "/exec/"+s.Id+"/start", true, ws, ws, ws, nil, nil); err != nil {
		panic(err)
	}
	fmt.Println("Connection!")
	fmt.Println(ws)
	spew.Dump(ws)
}

func hijack(addr, method, path string, setRawTerminal bool, in io.ReadCloser, stdout, stderr io.Writer, started chan io.Closer, data interface{}) error {

	params := bytes.NewBufferString("{\"Detach\": false, \"Tty\": true}")
	req, err := http.NewRequest(method, path, params)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Docker-Client")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")
	req.Host = addr

	dial, err := net.Dial("tcp", addr)
	// When we set up a TCP connection for hijack, there could be long periods
	// of inactivity (a long running command with no output) that in certain
	// network setups may cause ECONNTIMEOUT, leaving the client in an unknown
	// state. Setting TCP KeepAlive on the socket connection will prohibit
	// ECONNTIMEOUT unless the socket connection truly is broken
	if tcpConn, ok := dial.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}
	if err != nil {
		return err
	}
	clientconn := httputil.NewClientConn(dial, nil)
	defer clientconn.Close()

	// Server hijacks the connection, error 'connection closed' expected
	clientconn.Do(req)

	rwc, br := clientconn.Hijack()
	defer rwc.Close()

	if started != nil {
		started <- rwc
	}

	var receiveStdout chan error

	if stdout != nil || stderr != nil {
		go func() (err error) {
			if setRawTerminal && stdout != nil {
				_, err = io.Copy(stdout, br)
			}
			return err
		}()
	}

	go func() error {
		if in != nil {
			io.Copy(rwc, in)
		}

		if conn, ok := rwc.(interface {
			CloseWrite() error
		}); ok {
			if err := conn.CloseWrite(); err != nil {
			}
		}
		return nil
	}()

	if stdout != nil || stderr != nil {
		if err := <-receiveStdout; err != nil {
			return err
		}
	}
	spew.Dump(br)
	go func() {
		for {
			fmt.Println(br)
			spew.Dump(br)
		}
	}()

	return nil
}

var homeTemplate = template.Must(template.New("").Parse(`
<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta http-equiv="X-UA-Compatible" content="IE=edge">
<link href="//cdn.bootcss.com/bootstrap/3.2.0/css/bootstrap.min.css" rel="stylesheet">
<script src="//cdn.staticfile.org/jquery/1.11.1/jquery.min.js"></script>
<link rel="stylesheet" href="//cdn.staticfile.org/jqueryui/1.11.2/themes/smoothness/theme.css" />
<script src="//cdn.staticfile.org/jqueryui/1.11.2/jquery-ui.min.js"></script>
<script src="//cdn.staticfile.org/term.js/0.0.2/term.js"></script>
<script src="//cdn.bootcss.com/bootstrap/3.2.0/js/bootstrap.min.js"></script>
<style>
body {
    background-color: #000;
}

.terminal {
    border: #000 solid 5px;
    font-family: "DejaVu Sans Mono", "Liberation Mono", monospace;
    font-size: 14px;
    color: #f0f0f0;
    background: #000;
}

.terminal-cursor {
    color: #000;
    background: #f0f0f0;
}
</style>
</head>
<div style="color:red;font-size:16px;">请勿直接关闭浏览器：退出前，输入 'exit' 主动退出，避免容器产生 '/bin/sh' 进程 </div>
<div id="container-terminal"></div>
<script type="text/javascript">
$(function() {
    window.onbeforeunload=function(){
        return ("确认退出吗，确保已经在容器中exit?");
    }
    var websocket = new WebSocket("ws://" + window.location.hostname + ":" + window.location.port + "/exec?id={{.ContainerId}}");

    websocket.onopen = function(evt) {
        var term = new Terminal({
            cols: {{.Cols}},
            rows: {{.Rows}},
            screenKeys: true,
            useStyle: true,
            cursorBlink: true,
        });

        term.on('data', function(data) {
            websocket.send(data);
        });

        term.on('title', function(title) {
            document.title = title;
        });

        term.open(document.getElementById('container-terminal'));
    
        websocket.onmessage = function(evt) {
            term.write(evt.data);
        }

        websocket.onclose = function(evt) {
            term.write("Session terminated");
            term.destroy();
        }

        websocket.onerror = function(evt) {
            if (typeof console.log == "function") {
                console.log(evt)
            }
        }
    }
});
</script>
</html>
`))
