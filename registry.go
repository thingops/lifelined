package main;
import (
    "gopkg.in/redis.v5"
    "log"
    "time"
    "encoding/json"
    "github.com/rcrowley/go-metrics"
    "errors"
)

var lifeline_gauge metrics.Gauge

var redisClient redis.Cmdable = nil;

func initRegistry() {
    lifeline_gauge = metrics.NewGauge()
    metrics.Register("lifeline.lifelines", lifeline_gauge)
    var options, err  = redis.ParseURL(Config.RedisUrl)
    if err != nil {
        log.Panic(err);
    }

    if Config.RedisCluster {
        log.Println("connecting to redis cluster", Config.RedisUrl);
        redisClient = redis.NewClusterClient(&redis.ClusterOptions{
            Addrs: []string{options.Addr},
        })
        _,err = redisClient.Ping().Result()
        if err != nil {
            log.Panic(err);
        }
    } else {
        log.Println("connecting to", Config.RedisUrl);
        redisClient = redis.NewClient(options);
        _,err = redisClient.Ping().Result()
        if err != nil {
            log.Panic(err);
        }
    }

    go registryKeepAlive();
}

const REDIS_REG_EXPIRE =  time.Second * 20;

func registryKeepAlive() {
    for {
        time.Sleep(REDIS_REG_EXPIRE/2)
        log.Println(len(lifelines), "lifeline connections on this instance");
        lifeline_gauge.Update(int64(len(lifelines)))
        if connection_counter != nil {
            log.Println(connection_counter.Count(), "tcp connections");
        }
        for _, lf := range lifelines {
            time.Sleep(10)
            registerLifeline(lf, false);
        }
    }
}

func registerLifeline(lf *Lifeline, claim bool) {

    val, err := redisClient.HGet(lf.index, "lifelined").Result()
    if err == nil {
        if (val != Config.WanAddr) {
            if (claim) {
                log.Println(lf.index, "is locked by", val, ". overtaking because our connection is new");
            } else {
                log.Println(lf.index, "is locked by", val, ". our connection is probably dead");
                return;
            }
        }

    }
    b, _ := json.Marshal(lf.Properties)
    c  := lf.conn.RemoteAddr().String();

    m := map[string]string{
        "lifelined" : Config.WanAddr,
         "headers"  : string(b),
         "origin"   : c,
     }

    redisClient.Expire(lf.index, REDIS_REG_EXPIRE)
    redisClient.HMSet(lf.index, m)
}

func registryGet(name string) (map[string]interface{}, error) {
    var vv map[string]string;
    var err error;
    vv, err = redisClient.HGetAll(name).Result();
    if err != nil {
        return nil, err
    }
    var r = make(map[string]interface{})
    if len(vv) < 1 {
        return nil, errors.New("not found");
    }
    for k,v := range vv {
        var rr = make(map[string]string)
        err = json.Unmarshal([]byte(v), &rr)
        if err != nil {
            r[k] = v;
        } else {
            r[k] = rr;
        }
    }
    return r, nil
}
