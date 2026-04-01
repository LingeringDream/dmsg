package store

import (
"encoding/json"
"github.com/dgraph-io/badger/v3"
)

type Message struct {
ID        string `json:"id"`
PubKey    string `json:"pubkey"`
Content   string `json:"content"`
Timestamp int64  `json:"timestamp"`
}

type Service struct {
db *badger.DB
}

func New(path string) (*Service, error) {
	opts := badger.DefaultOptions(path)
db, err := badger.Open(opts)
if err != nil {
return nil, err
}
return &Service{db: db}, nil
}

func (s *Service) Put(msg *Message) error {
data, err := json.Marshal(msg)
if err != nil {
return err
}
return s.db.Update(func(txn *badger.Txn) error {
return txn.Set([]byte(msg.ID), data)
})
}

func (s *Service) Get(id string) (*Message, error) {
var msg Message
err := s.db.View(func(txn *badger.Txn) error {
item, err := txn.Get([]byte(id))
if err != nil {
return err
}
return item.Value(func(val []byte) error {
return json.Unmarshal(val, &msg)
})
})
return &msg, err
}

func (s *Service) Close() {
s.db.Close()
}
