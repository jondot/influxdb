package server

import (
	"admin"
	"api/http"
	"configuration"
	"coordinator"
	"datastore"
	"engine"
	"log"
)

type Server struct {
	RaftServer     *coordinator.RaftServer
	Db             datastore.Datastore
	ProtobufServer *coordinator.ProtobufServer
	ClusterConfig  *coordinator.ClusterConfiguration
	HttpApi        *http.HttpServer
	AdminServer    *admin.HttpServer
	Coordinator    coordinator.Coordinator
	Config         *configuration.Configuration
}

func NewServer(config *configuration.Configuration) (*Server, error) {
	log.Println("Opening database at ", config.DataDir)
	db, err := datastore.NewLevelDbDatastore(config.DataDir)
	if err != nil {
		return nil, err
	}

	clusterConfig := coordinator.NewClusterConfiguration()
	raftServer := coordinator.NewRaftServer(config.RaftDir, "localhost", config.RaftServerPort, clusterConfig)
	requestHandler := coordinator.NewProtobufRequestHandler(db, raftServer)
	protobufServer := coordinator.NewProtobufServer(config.ProtobufPortString(), requestHandler)
	coord := coordinator.NewCoordinatorImpl(db, raftServer, clusterConfig)

	eng, err := engine.NewQueryEngine(coord)
	if err != nil {
		return nil, err
	}

	httpApi := http.NewHttpServer(config.ApiHttpPortString(), eng, coord, coord)

	return &Server{
		RaftServer:     raftServer,
		Db:             db,
		ProtobufServer: protobufServer,
		ClusterConfig:  clusterConfig,
		HttpApi:        httpApi,
		Coordinator:    coord,
		Config:         config}, nil
}

func (self *Server) ListenAndServe() error {
	go self.ProtobufServer.ListenAndServe()

	retryUntilJoinedCluster := false
	if len(self.Config.SeedServers) > 0 {
		retryUntilJoinedCluster = true
	}
	go self.RaftServer.ListenAndServe(self.Config.SeedServers, retryUntilJoinedCluster)
	log.Println("Starting admin interface on port", self.Config.AdminHttpPort)
	go self.AdminServer.ListenAndServe()
	log.Println("Starting Http Api server on port", self.Config.ApiHttpPort)
	self.HttpApi.ListenAndServe()
	return nil
}
