package coordinator

import (
	"datastore"
	"fmt"
	. "launchpad.net/gocheck"
	"os"
	"protocol"
	"time"
)

type ClientServerSuite struct{}

var _ = Suite(&ClientServerSuite{})

const DB_DIR = "/tmp/influxdb/datastore_test"

func newDatastore(c *C) datastore.Datastore {
	os.MkdirAll(DB_DIR, 0744)
	db, err := datastore.NewLevelDbDatastore(DB_DIR)
	c.Assert(err, Equals, nil)
	return db
}

func cleanDb(db datastore.Datastore) {
	if db != nil {
		db.Close()
	}
	os.RemoveAll(DB_DIR)
}

func (self *ClientServerSuite) TestClientCanMakeRequests(c *C) {
	server := startAndVerifyCluster(1, c)[0]
	defer clean(server)
	db := newDatastore(c)
	requestHandler := NewProtobufRequestHandler(db, server)
	protobufServer := NewProtobufServer(":8091", requestHandler)
	go protobufServer.ListenAndServe()
	c.Assert(protobufServer, Not(IsNil))
	protobufClient := NewProtobufClient("localhost:8091")
	responseStream := make(chan *protocol.Response, 1)

	mock := `
  {
    "points": [
      { "values": [{"int64_value": 3}]},
      { "values": [{"int64_value": 23}]}
    ],
    "name": "foo",
    "fields": ["val"]
  }`
	fmt.Println("creating series")
	series := stringToSeries(mock, c)
	id := uint32(1)
	database := "pauldb"
	proxyWrite := protocol.Request_PROXY_WRITE
	request := &protocol.Request{Id: &id, Type: &proxyWrite, Database: &database, Series: series}

	time.Sleep(time.Second * 1)
	err := protobufClient.MakeRequest(request, responseStream)
	c.Assert(err, IsNil)
	timer := time.NewTimer(time.Second)
	select {
	case <-timer.C:
		c.Error("Timed out waiting for response")
	case response := <-responseStream:
		c.Assert(*response.Type, Equals, protocol.Response_WRITE_OK)
	}
}

func (self *ClientServerSuite) TestClientReconnectsIfDisconnected(c *C) {
}

func (self *ClientServerSuite) TestServerExecutesReplayRequestIfWriteIsOutOfSequence(c *C) {
}

func (self *ClientServerSuite) TestServerKillsOldHandlerWhenClientReconnects(c *C) {

}
