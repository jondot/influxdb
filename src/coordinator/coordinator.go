package coordinator

import (
	"common"
	"datastore"
	"fmt"
	"parser"
	"protocol"
)

type CoordinatorImpl struct {
	clusterConfiguration *ClusterConfiguration
	raftServer           ClusterConsensus
	datastore            datastore.Datastore
}

func NewCoordinatorImpl(datastore datastore.Datastore, raftServer ClusterConsensus, clusterConfiguration *ClusterConfiguration) *CoordinatorImpl {
	return &CoordinatorImpl{
		clusterConfiguration: clusterConfiguration,
		raftServer:           raftServer,
		datastore:            datastore,
	}
}

func (self *CoordinatorImpl) DistributeQuery(user common.User, db string, query *parser.Query, yield func(*protocol.Series) error) error {
	return self.datastore.ExecuteQuery(user, db, query, yield)
}

func (self *CoordinatorImpl) WriteSeriesData(user common.User, db string, series *protocol.Series) error {
	if !user.HasWriteAccess(db) {
		return fmt.Errorf("Insufficient permission to write to %s", db)
	}

	return self.datastore.WriteSeriesData(db, series)
}

func (self *CoordinatorImpl) CreateDatabase(user common.User, db string, replicationFactor uint8) error {
	if !user.IsClusterAdmin() {
		return fmt.Errorf("Insufficient permission to create database")
	}

	err := self.raftServer.CreateDatabase(db, replicationFactor)
	if err != nil {
		return err
	}
	return nil
}

func (self *CoordinatorImpl) ListDatabases(user common.User) ([]*Database, error) {
	if !user.IsClusterAdmin() {
		return nil, fmt.Errorf("Insufficient permission to list databases")
	}

	dbs := self.clusterConfiguration.GetDatabases()
	return dbs, nil
}

func (self *CoordinatorImpl) DropDatabase(user common.User, db string) error {
	if !user.IsClusterAdmin() {
		return fmt.Errorf("Insufficient permission to drop database")
	}

	return self.raftServer.DropDatabase(db)
}

func (self *CoordinatorImpl) AuthenticateDbUser(db, username, password string) (common.User, error) {
	dbUsers := self.clusterConfiguration.dbUsers[db]
	if dbUsers == nil || dbUsers[username] == nil {
		return self.AuthenticateClusterAdmin(username, password)
	}
	user := dbUsers[username]
	if user.isValidPwd(password) {
		return user, nil
	}
	return nil, fmt.Errorf("Invalid username/password")
}

func (self *CoordinatorImpl) AuthenticateClusterAdmin(username, password string) (common.User, error) {
	user := self.clusterConfiguration.clusterAdmins[username]
	if user == nil {
		return nil, fmt.Errorf("Invalid username/password")
	}
	if user.isValidPwd(password) {
		return user, nil
	}
	return nil, fmt.Errorf("Invalid username/password")
}

func (self *CoordinatorImpl) ListClusterAdmins(requester common.User) ([]string, error) {
	if !requester.IsClusterAdmin() {
		return nil, fmt.Errorf("Insufficient permissions")
	}

	return self.clusterConfiguration.GetClusterAdmins(), nil
}

func (self *CoordinatorImpl) CreateClusterAdminUser(requester common.User, username string) error {
	if !requester.IsClusterAdmin() {
		return fmt.Errorf("Insufficient permissions")
	}

	if self.clusterConfiguration.clusterAdmins[username] != nil {
		return fmt.Errorf("User %s already exists", username)
	}

	return self.raftServer.SaveClusterAdminUser(&clusterAdmin{CommonUser{Name: username}})
}

func (self *CoordinatorImpl) DeleteClusterAdminUser(requester common.User, username string) error {
	if !requester.IsClusterAdmin() {
		return fmt.Errorf("Insufficient permissions")
	}

	user := self.clusterConfiguration.clusterAdmins[username]
	if user == nil {
		return fmt.Errorf("User %s doesn't exists", username)
	}

	user.CommonUser.IsUserDeleted = true
	return self.raftServer.SaveClusterAdminUser(user)
}

func (self *CoordinatorImpl) ChangeClusterAdminPassword(requester common.User, username, password string) error {
	if !requester.IsClusterAdmin() {
		return fmt.Errorf("Insufficient permissions")
	}

	user := self.clusterConfiguration.clusterAdmins[username]
	if user == nil {
		return fmt.Errorf("Invalid user name %s", username)
	}

	user.changePassword(password)
	return self.raftServer.SaveClusterAdminUser(user)
}

func (self *CoordinatorImpl) CreateDbUser(requester common.User, db, username string) error {
	if !requester.IsClusterAdmin() && !requester.IsDbAdmin(db) {
		return fmt.Errorf("Insufficient permissions")
	}

	self.clusterConfiguration.CreateDatabase(db, uint8(1)) // ignore the error since the db may exist
	dbUsers := self.clusterConfiguration.dbUsers[db]
	if dbUsers != nil && dbUsers[username] != nil {
		return fmt.Errorf("User %s already exists", username)
	}

	if dbUsers == nil {
		dbUsers = map[string]*dbUser{}
		self.clusterConfiguration.dbUsers[db] = dbUsers
	}

	matchers := []*Matcher{&Matcher{true, ".*"}}
	return self.raftServer.SaveDbUser(&dbUser{CommonUser{Name: username}, db, matchers, matchers, false})
}

func (self *CoordinatorImpl) DeleteDbUser(requester common.User, db, username string) error {
	if !requester.IsClusterAdmin() && !requester.IsDbAdmin(db) {
		return fmt.Errorf("Insufficient permissions")
	}

	dbUsers := self.clusterConfiguration.dbUsers[db]
	if dbUsers == nil || dbUsers[username] == nil {
		return fmt.Errorf("User %s doesn't exists", username)
	}

	user := dbUsers[username]
	user.CommonUser.IsUserDeleted = true
	return self.raftServer.SaveDbUser(user)
}

func (self *CoordinatorImpl) ListDbUsers(requester common.User, db string) ([]string, error) {
	if !requester.IsClusterAdmin() && !requester.IsDbAdmin(db) {
		return nil, fmt.Errorf("Insufficient permissions")
	}

	return self.clusterConfiguration.GetDbUsers(db), nil
}

func (self *CoordinatorImpl) ChangeDbUserPassword(requester common.User, db, username, password string) error {
	if !requester.IsClusterAdmin() && !requester.IsDbAdmin(db) && !(requester.GetDb() == db && requester.GetName() == username) {
		return fmt.Errorf("Insufficient permissions")
	}

	dbUsers := self.clusterConfiguration.dbUsers[db]
	if dbUsers == nil || dbUsers[username] == nil {
		return fmt.Errorf("Invalid username %s", username)
	}

	dbUsers[username].changePassword(password)
	return self.raftServer.SaveDbUser(dbUsers[username])
}

func (self *CoordinatorImpl) SetDbAdmin(requester common.User, db, username string, isAdmin bool) error {
	if !requester.IsClusterAdmin() && !requester.IsDbAdmin(db) {
		return fmt.Errorf("Insufficient permissions")
	}

	dbUsers := self.clusterConfiguration.dbUsers[db]
	if dbUsers == nil || dbUsers[username] == nil {
		return fmt.Errorf("Invalid username %s", username)
	}

	user := dbUsers[username]
	user.IsAdmin = isAdmin
	self.raftServer.SaveDbUser(user)
	return nil
}
