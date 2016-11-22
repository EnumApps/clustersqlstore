/* Gorilla Sessions backend for ClusterSQL.

Copyright (c) 2016 Contributors. See the list of contributors in the CONTRIBUTORS file for details.

This software is licensed under a MIT style license available in the LICENSE file.
*/
package clustersqlstore

import (
	"database/sql"
	"encoding/gob"
	"enumapps/console"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/EnumApps/aerror"
	_ "github.com/EnumApps/clustersql"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

const (
	//use more const instead of var
	tableNameSession = "`session_cluster`"
	insQ             = "INSERT INTO " + tableNameSession +
		"(`id`, `data`, `expire_on`) VALUES (?, ?, ?)"
	delQ = "DELETE FROM " + tableNameSession + " WHERE `id` = ?"
	updQ = "UPDATE " + tableNameSession + " SET `data` = ?, `expire_on` = ? WHERE `id` = ?"
	selQ = "SELECT `data`, `expire_on` FROM " + tableNameSession + " WHERE `id` = ? LIMIT 1"
	//speical fields
	// fieldCreate = "c"	//not even need to expose
	// fieldModify = "m"	//not even need to expose
	fieldExpire = "x"

	mysqlTimeFormat = "2006-01-02 15:04:05"
)

type ClusterSQLStore struct {
	db         *sql.DB
	stmtInsert *sql.Stmt
	stmtDelete *sql.Stmt
	stmtUpdate *sql.Stmt
	stmtSelect *sql.Stmt

	Codecs  []securecookie.Codec
	Options *sessions.Options
}

type sessionRow struct {
	id   string
	data string
	m    time.Time
	x    time.Time
}

// may use this to replace the value of create time, optional
// func (sr *sessionRow) c() time.Time {
// }

func init() {
	rand.Seed(time.Now().UnixNano())
	gob.Register(time.Time{})
}

//NewClusterSQLStore return a ClusterSQLStore, driverName is the name of preregistered cluster drvier
func NewClusterSQLStore(driverName, path string, maxAge int, keyPairs ...[]byte) (*ClusterSQLStore, error) {
	db, err := sql.Open(driverName, "")
	if err != nil {
		return nil, err
	}

	return NewClusterSQLStoreConnection(db, path, maxAge, keyPairs...)
}

//NewClusterSQLStore return a ClusterSQLStore, db is an existing db connection
func NewClusterSQLStoreConnection(db *sql.DB, path string, maxAge int, keyPairs ...[]byte) (*ClusterSQLStore, error) {

	stmtInsert, stmtErr := db.Prepare(insQ)
	if stmtErr != nil {
		return nil, aerror.WrapError(stmtErr)
	}

	stmtDelete, stmtErr := db.Prepare(delQ)
	if stmtErr != nil {
		return nil, aerror.WrapError(stmtErr)
	}

	stmtUpdate, stmtErr := db.Prepare(updQ)
	if stmtErr != nil {
		return nil, aerror.WrapError(stmtErr)
	}
	stmtSelect, stmtErr := db.Prepare(selQ)
	if stmtErr != nil {
		return nil, aerror.WrapError(stmtErr)
	}

	return &ClusterSQLStore{
		db:         db,
		stmtInsert: stmtInsert,
		stmtDelete: stmtDelete,
		stmtUpdate: stmtUpdate,
		stmtSelect: stmtSelect,
		Codecs:     securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   path,
			MaxAge: maxAge,
		},
	}, nil
}

//Close implement the Close method of session store
func (m *ClusterSQLStore) Close() {
	m.stmtSelect.Close()
	m.stmtUpdate.Close()
	m.stmtDelete.Close()
	m.stmtInsert.Close()
	m.db.Close()
}

//Get implement the Get method of session store
func (m *ClusterSQLStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(m, name)
}

//New implement the New method of session store
func (m *ClusterSQLStore) New(r *http.Request, name string) (*sessions.Session, error) {
	session := sessions.NewSession(m, name)
	session.Options = &sessions.Options{
		Path:   m.Options.Path,
		MaxAge: m.Options.MaxAge,
	}
	session.IsNew = true
	var err error
	if cook, errCookie := r.Cookie(name); errCookie == nil {
		err = securecookie.DecodeMulti(name, cook.Value, &session.ID, m.Codecs...)
		if err == nil {
			err = m.load(session)
			if err == nil {
				session.IsNew = false
			} else {
				err = nil
			}
		}
	}
	return session, err
}

//Save implement the Save method of session store
func (m *ClusterSQLStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	var err error
	if session.ID == "" {
		if err = m.insert(session); err != nil {
			return err
		}
	} else if err = m.save(session); err != nil {
		return err
	}
	encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, m.Codecs...)
	if err != nil {
		return err
	}
	http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	return nil
}

func (m *ClusterSQLStore) insert(session *sessions.Session) error {
	ct := time.Now()
	id := ct.Format(time.RFC3339Nano) + strconv.Itoa(rand.Intn(89999)+10000)

	var x time.Time
	exOn := session.Values[fieldExpire]
	if exOn == nil {
		x = time.Now().Add(time.Second * time.Duration(session.Options.MaxAge))
	} else {
		x = exOn.(time.Time)
	}
	// delete(session.Values, fieldCreate)//why need to expose
	delete(session.Values, fieldExpire)
	// delete(session.Values, fieldModify)//why need to expose

	encoded, encErr := securecookie.EncodeMulti(session.Name(), session.Values, m.Codecs...)
	if encErr != nil {
		return encErr
	}
	_, insErr := m.stmtInsert.Exec(id, encoded, x.Format(mysqlTimeFormat))
	if insErr != nil {
		return insErr
	}
	session.ID = id
	return nil
}

//Delete allow delete of mysql session (not exposed by gorilla sessions interface).
func (m *ClusterSQLStore) Delete(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {

	// Set cookie to expire.
	options := *session.Options
	options.MaxAge = -1
	http.SetCookie(w, sessions.NewCookie(session.Name(), "", &options))
	// Clear session values.
	for k := range session.Values {
		delete(session.Values, k)
	}

	_, delErr := m.stmtDelete.Exec(session.ID)
	if delErr != nil {
		return delErr
	}
	return nil
}

func (m *ClusterSQLStore) save(session *sessions.Session) error {
	fmt.Println("SAVE>>>", session.Values)
	if session.IsNew == true {
		return m.insert(session)
	}
	var x, ct time.Time
	//create time removed, it shall never change
	ct = time.Now() //ct is current time, stable through the whole method

	exOn := session.Values[fieldExpire]
	if exOn == nil {
		x = time.Now().Add(time.Second * time.Duration(session.Options.MaxAge))
	} else {
		x = exOn.(time.Time)
		if x.Sub(ct.Add(time.Second*time.Duration(session.Options.MaxAge))) < 0 {
			x = ct.Add(time.Second * time.Duration(session.Options.MaxAge))
		}
	}

	delete(session.Values, fieldExpire)
	encoded, encErr := securecookie.EncodeMulti(session.Name(), session.Values, m.Codecs...)
	if encErr != nil {
		return encErr
	}
	_, updErr := m.stmtUpdate.Exec(encoded, x, session.ID)
	if updErr != nil {
		return updErr
	}
	return nil
}

func (m *ClusterSQLStore) load(session *sessions.Session) error {
	row := m.stmtSelect.QueryRow(session.ID)
	sess := sessionRow{}
	var sx string
	scanErr := row.Scan(&sess.data, &sx)
	if scanErr != nil {
		return aerror.WrapError(scanErr)
	}
	x, err := time.Parse(mysqlTimeFormat, sx)
	if err != nil {
		return aerror.WrapError(err) //shall not happen, must trace
	}
	sess.x = x
	if sess.x.Sub(time.Now()) < 0 {
		// log.Printf("Session expired on %s, but it is %s now.", sess.expiresOn, time.Now())
		return aerror.New("Session expired")
	}
	err = securecookie.DecodeMulti(session.Name(), sess.data, &session.Values, m.Codecs...)
	if err != nil {
		console.CInfo(session.Values, err, sess.data)
		return aerror.WrapError(err)
	}
	session.Values[fieldExpire] = sess.x

	return nil
}
