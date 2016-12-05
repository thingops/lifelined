package main;

import (
    "net/http"
    "github.com/bugsnag/bugsnag-go"
    "github.com/gorilla/websocket"
    "github.com/gorilla/mux"
    "github.com/gorilla/handlers"
    "os"
    "log"
    "encoding/json"
    "github.com/rs/cors"
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}


func wsProxy(from*websocket.Conn, to *websocket.Conn) {
    defer from.Close();
    defer to.Close();
    for {
        messageType, bytes, err := from.ReadMessage();
        if err != nil {
            break
        }
        to.WriteMessage(messageType, bytes)
    }
}

func remoteSshLifelineHandler(name string, w http.ResponseWriter, conn *websocket.Conn) {
    var lf, err = registryGet(name)
    if (err != nil){
        conn.WriteMessage(websocket.TextMessage, []byte("lifeline " + name  + " not registered."))
        conn.Close()
        return;
    }

    var lifelined = lf["lifelined"].(string);
    if (lifelined == Config.WanAddr) {
        conn.WriteMessage(websocket.TextMessage, []byte(name + " is registered at myself, but i don't have it. please try again later."))
        conn.Close()
        return;
    }
    conn.WriteMessage(websocket.TextMessage, []byte(name + " is registered at " + lf["lifelined"].(string)  + ". rerouting you there now."))

    c, _, err := websocket.DefaultDialer.Dial("ws://" + lf["lifelined"].(string) + "/lifelines/" + name + "/ssh", nil)
    if err != nil {
        log.Println("dial:", err)
        conn.WriteMessage(websocket.TextMessage, []byte("other lifelined did not respond. try again later"))
        return;
    }

    go wsProxy(conn, c);
    go wsProxy(c, conn);
}


func wsWriter(conn *websocket.Conn, lf *Lifeline) {
    defer conn.Close();
    lf.Enable();
    for {
        b := <-lf.cx
        if (len(b) < 1) {
            break;
        }
        conn.WriteMessage(websocket.BinaryMessage, b)
    }
}
func wsReader(conn *websocket.Conn, lf *Lifeline) {
    defer conn.Close();
    defer lf.Release(true);
    for {
        messageType, bytes, err := conn.ReadMessage();
        if err != nil {
            break
        }
        if messageType == websocket.BinaryMessage {
            lf.conn.Write(bytes)
        }
    }
}
func sshLifelineHandler(w http.ResponseWriter, r *http.Request) {
    var vars = mux.Vars(r)
    var name = vars["name"]

    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println(err)
        return
    }
    conn.WriteMessage(websocket.TextMessage, []byte("helo. this is " + Config.WanAddr))

    lifelinesMutex.Lock()
    var lf *Lifeline = lifelines[name]
    lifelinesMutex.Unlock()

    if (lf == nil) {
        remoteSshLifelineHandler(name, w, conn);
        return
    }
    conn.WriteMessage(websocket.TextMessage, []byte("your lifeline is local. dispatching now."))

    go wsReader(conn, lf);
    go wsWriter(conn, lf);
}

func getLifelineHandler(w http.ResponseWriter, r *http.Request) {
    var vars = mux.Vars(r)
    var name = vars["name"]

    var lf, err = registryGet(name)
    if err != nil {
        log.Println(err)
        w.WriteHeader(404);
        return;
    }

    _, local := lifelines[name]

    json.NewEncoder(w).Encode(struct{
        Self        string `json:"self"`
        Lifeline    map[string]interface{} `json:"lifeline"`
        Local       bool `json:"local"`
    }{
        Config.WanAddr,
        lf,
        local,
    })

    if err != nil {
        log.Println(err)
        w.WriteHeader(500);
        return;
    }
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(200);
    keys := make([]string, len(lifelines))
    i := 0
    for k := range lifelines {
        keys[i] = k
        i++
    }

    json.NewEncoder(w).Encode(struct{
        Self string `json:"self"`
        Locals []string `json:"locals"`
    }{
        Config.WanAddr,
        keys,
    })
}

func ApiServer() {
    corses := cors.New(cors.Options{
        AllowedOrigins: []string{"*"},
        AllowCredentials: true,
        AllowedMethods: []string{"GET", "OPTIONS"},
        AllowedHeaders: []string{"accept", "authorization", "Content-Type"},
        Debug: true,
    })
    r := mux.NewRouter()
    r.HandleFunc("/",  statusHandler)
    r.HandleFunc("/lifelines/{name}",     getLifelineHandler)
    r.HandleFunc("/lifelines/{name}/ssh", sshLifelineHandler)
    log.Println("apiserver on", Config.ApiAddr());

    log.Fatal(http.ListenAndServe(Config.ApiAddr(), bugsnag.Handler(
        handlers.LoggingHandler(os.Stderr,
        corses.Handler(
            r)))));
}

