package main

import (
	"errors"
	"log"
	"net"
	"strconv"
	"strings"

	common "../../proto/common"
	discover "../../proto/discover"
	util "../../util"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	redis "gopkg.in/redis.v5"
)

const (
	port = ":50054"
)

type server struct{}

type Server struct {
	host string
	port int32
}

func parseServer(name string) (Server, error) {
	var srv Server
	vals := strings.Split(name, ":")
	if len(vals) != 2 {
		log.Printf("length:%d", len(vals))
		return srv, errors.New("parse failed")
	}
	port, err := strconv.Atoi(vals[1])
	if err != nil {
		log.Printf("strconv failed, %s:%v", vals[1], err)
		return srv, err
	}
	srv.host = vals[0]
	srv.port = int32(port)
	return srv, nil
}

func fetchServers(name string) []Server {
	client := util.InitRedis()
	vals, err := client.ZRangeByScore(name, redis.ZRangeBy{Min: "-inf", Max: "+inf", Offset: 0, Count: 10}).Result()
	if err != nil {
		log.Printf("zrangebyscore failed %s:%v", name, err)
		return nil
	}

	servers := make([]Server, 10)
	var idx int
	for i, key := range vals {
		idx = i
		log.Printf("%d:%s", i, key)
		srv, err := parseServer(key)
		if err != nil {
			log.Printf("parse failed:%v", err)
			continue
		}
		servers[i] = srv
		if i >= 10 {
			break
		}
	}

	return servers[:idx+1]
}

func (s *server) Resolve(ctx context.Context, in *discover.ServerRequest) (*discover.ServerReply, error) {
	log.Printf("resolve request uid:%d server:%s", in.Head.Uid, in.Sname)
	servers := fetchServers(in.Sname)
	if len(servers) == 0 {
		log.Printf("fetch servers failed:%s", in.Sname)
		return &discover.ServerReply{Head: &common.Head{Retcode: common.ErrCode_FETCH_SERVER}}, nil
	}
	srv := servers[util.Randn(int32(len(servers)))]
	return &discover.ServerReply{Head: &common.Head{Retcode: 0, Uid: in.Head.Uid}, Host: srv.host, Port: srv.port}, nil
}

func main() {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	discover.RegisterDiscoverServer(s, &server{})
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}