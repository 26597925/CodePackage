package main

import "os"
import "fmt"
import "log"
import "bytes"
import "encoding/binary"

const HEADER_SIZE = 32
const MAGIC = 0x494d494d
const VERSION = 1<<16 //1.0

const OFFLINE = "offline"

//离线消息存储
type Storage struct {
    files map[int64]*os.File
    ic chan *IMMessage 
    cc chan int64
    root string
}

func NewStorage() *Storage {
    storage := new(Storage)
    storage.ic = make(chan *IMMessage)
    storage.cc = make(chan int64)
    storage.files = make(map[int64]*os.File)
    storage.root = "/tmp"
    path := fmt.Sprintf("%s/%s", storage.root, OFFLINE)
    err := os.Mkdir(path, 0755)
    if err != nil && !os.IsExist(err) {
        panic("mkdir error")
    }
    return storage
}

func (storage *Storage) Start() {
    go storage.Run()
}

func (storage *Storage) SaveOfflineMessage(message *IMMessage) {
    storage.ic <- message
    log.Println("save off line message")
}

func (storage *Storage) ClearOfflineMessage(uid int64) {
    storage.cc <- uid
}

func (storage *Storage) LoadOfflineMessage(uid int64) chan *IMMessage {
    path := storage.GetOfflinePath(uid)
    file, err := os.Open(path)
    if err != nil {
        return nil
    }

    fi, err := file.Stat()
    if err != nil {
        return nil
    }
    if fi.Size() <= HEADER_SIZE {
        return nil
    }
    
    magic, version := storage.ReadHeader(file)
    if magic != MAGIC {
        log.Println("magic unmatch")
        return nil
    }
    if version != VERSION {
        log.Println("version unknown")
        return nil
    }

    c := make(chan *IMMessage)
    go func() {
        for {
            msg := storage.ReadMessage(file)
            if msg == nil {
                break
            }
            c <- msg
        }
        close(c)
    }()
    return c
}


func (storage *Storage) GetOfflinePath(uid int64) string {
    return fmt.Sprintf("%s/%s/%d", storage.root, OFFLINE, uid)
}

func (storage *Storage) ReadMessage(file *os.File) *IMMessage {
    var size int32
    err := binary.Read(file, binary.BigEndian, &size)
    if err != nil {
        return nil
    }
    if size < 16 {
        return nil
    }
    var sender int64
    var receiver int64
    err = binary.Read(file, binary.BigEndian, &sender)
    if err != nil {
        return nil
    }
    
    err = binary.Read(file, binary.BigEndian, &receiver)
    if err != nil {
        return nil
    }
    
    buf := make([]byte, size - 16)
    n, err := file.Read(buf)
    if err != nil || n != int(size - 16) {
        return nil
    }
    return &IMMessage{sender, receiver, string(buf)}
}

func (storage *Storage) WriteMessage(file *os.File, message *IMMessage) {
    var size int32 = int32(16 + len(message.content))
    err := binary.Write(file, binary.BigEndian, size)
    if err != nil {
        log.Println("file:", file)
        log.Panicln(err)
    }
    err = binary.Write(file, binary.BigEndian, message.sender)
    if err != nil {
        log.Panicln(err)
    }
    err = binary.Write(file, binary.BigEndian, message.receiver)
    if err != nil {
        log.Panicln(err)
    }
    n, err := file.Write([]byte(message.content))
    if err != nil || n != len(message.content) {
        log.Panicln(err)
    }
}

func (storage *Storage) ReadHeader(file *os.File) (magic int, version int) {
    header := make([]byte, HEADER_SIZE)
    n, err := file.Read(header)
    if err != nil || n != HEADER_SIZE {
        return
    }
    buffer := bytes.NewBuffer(header)
    var m, v int32
    binary.Read(buffer, binary.BigEndian, &m)
    binary.Read(buffer, binary.BigEndian, &v)
    magic = int(m)
    version = int(v)
    return
}

func (storage *Storage) WriteHeader(file *os.File) {
    var m int32 = MAGIC
    err := binary.Write(file, binary.BigEndian, m)
    if err != nil {
        log.Panicln(err)
    }
    var v int32 = VERSION
    err = binary.Write(file, binary.BigEndian, v)
    if err != nil {
        log.Panicln(err)
    }
    pad := make([]byte, HEADER_SIZE - 8)
    n, err := file.Write(pad)
    if err != nil || n != (HEADER_SIZE-8) {
        log.Panicln(err)
    }
}

//保存离线消息
func (storage *Storage) SaveMessage(msg *IMMessage) {
    _, ok := storage.files[msg.receiver]
    if !ok {
        path := storage.GetOfflinePath(msg.receiver)
        fmt.Println("path:", path)
        file , err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
        if err != nil {
            log.Panic("open file")
        }
        file_size, err := file.Seek(0, os.SEEK_END)
        if err != nil {
            log.Panic("seek file")
        }
        if file_size < HEADER_SIZE && file_size > 0 {
            log.Println("file header is't complete")
            err = file.Truncate(0)
            if err != nil {
                log.Panic("truncate file")
            }
            file_size = 0
        }
        if file_size == 0 {
            storage.WriteHeader(file)
        }
        storage.files[msg.receiver] = file
    }
    storage.WriteMessage(storage.files[msg.receiver], msg)
}

//清空离线消息
func (storage *Storage) ClearMessage(uid int64) {
    file, ok := storage.files[uid]
    if ok {
        delete(storage.files, uid)
        file.Close()
    }
    path := storage.GetOfflinePath(uid)
    os.Remove(path)    
}

func (storage *Storage) Run() {
    for {
        select {
        case msg := <- storage.ic:
            storage.SaveMessage(msg)
        case uid := <- storage.cc:
            storage.ClearMessage(uid)
        }
    }
}
