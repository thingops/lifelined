package main;

import (
    "time"
    "github.com/rcrowley/go-metrics"
    "github.com/vrischmann/go-metrics-influxdb"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/aws/ec2metadata"
    "github.com/bugsnag/bugsnag-go"
    "strconv"
    "log"
    "os"
    "github.com/caarlos0/env"
)

type ConfigS struct {
    Bind          string  `env:"BIND"`
    ApiPort       int     `env:"PORT"  envDefault:"8080"`
    LFPort        int     `env:"LEGACY_PORT" envDefault:"8000"`
    RedisUrl      string  `env:"REDIS_URL" envDefault:"redis://localhost:6379/"`
    RedisCluster  bool    `env:"REDIS_CLUSTER"`
    WanAddr       string  `env:"WAN_ADDR"`
    BugsnagApiKey string  `env:"BUGSNAG_API_KEY"`
}
var Config = &ConfigS{}

func (c *ConfigS) ApiAddr() string {
    return c.Bind + ":" + strconv.Itoa(c.ApiPort);
}

func (c *ConfigS) LFAddr() string {
    return c.Bind + ":" + strconv.Itoa(c.LFPort);
}

func getWanAddr() {
    sess, err := session.NewSession()
    if err != nil {
        panic(err)
    }

    if len(Config.WanAddr) < 1 {
        if len(Config.Bind) > 0 {
            Config.WanAddr = Config.ApiAddr();
        } else {
            log.Println("resolving bind address through ec2 api");
            e2m   := ec2metadata.New(sess)
            ec2id,err  := e2m.GetInstanceIdentityDocument();
            if err != nil {
                log.Println("GetInstanceIdentityDocument failed. Specify BIND= env if not on aws!")
                log.Panic(err)
                return
            }
            Config.WanAddr = ec2id.PrivateIP + Config.ApiAddr()
        }
    }
    log.Println("registry wan", Config.WanAddr);
}

func main() {
    err := env.Parse(Config)
    if (err != nil) {
        log.Panic(err);
    }

    getWanAddr();
    hostname, err := os.Hostname()
    if err != nil {
        panic(err)
    }

    bugsnag.Configure(bugsnag.Configuration{
        APIKey: Config.BugsnagApiKey,
    })

    reg := metrics.NewPrefixedChildRegistry(metrics.DefaultRegistry, "lifeline.");
    metrics.RegisterDebugGCStats(reg)
    metrics.RegisterRuntimeMemStats(reg)
    go metrics.CaptureDebugGCStats(reg, time.Second*5)
    go metrics.CaptureRuntimeMemStats(reg, time.Second*5)

    go influxdb.InfluxDBWithTags(
        metrics.DefaultRegistry, // metrics registry
        time.Second * 10,        // interval
        "http://influxdb-monitor.superscale.io:8086/", // the InfluxDB url
        "monitor",                  // your InfluxDB database
        "myuser",                // your InfluxDB user
        "mypassword",            // your InfluxDB password
        map[string]string{"host": hostname},
    )

    lifelines = make(map[string]*Lifeline)

    initRegistry();
    go LifeLineServer()
    ApiServer()
}
