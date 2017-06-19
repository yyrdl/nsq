package nsqd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

const (
	MsgIDLength       = 16
	minValidMsgLength = MsgIDLength + 8 + 2 // Timestamp + Attempts
)

type MessageID [MsgIDLength]byte

type Message struct {
	ID        MessageID
	Body      []byte
	Timestamp int64
	Attempts  uint16

	// for in-flight handling
	deliveryTS time.Time
	clientID   int64
	pri        int64
	index      int
	deferred   time.Duration
}

func NewMessage(id MessageID, body []byte) *Message {
	return &Message{
		ID:        id,
		Body:      body,
		Timestamp: time.Now().UnixNano(),
	}
}

func (m *Message) WriteTo(w io.Writer) (int64, error) {
	var buf [10]byte
	var total int64

	binary.BigEndian.PutUint64(buf[:8], uint64(m.Timestamp))
	binary.BigEndian.PutUint16(buf[8:10], uint16(m.Attempts))

	n, err := w.Write(buf[:])
	total += int64(n)
	if err != nil {
		return total, err
	}

	n, err = w.Write(m.ID[:])
	total += int64(n)
	if err != nil {
		return total, err
	}

	n, err = w.Write(m.Body)
	total += int64(n)
	if err != nil {
		return total, err
	}

	return total, nil
}

// decodeMessage deserializes data (as []byte) and creates a new Message
// message format:
// [x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x][x]...
// |       (int64)        ||    ||      (hex string encoded in ASCII)           || (binary)
// |       8-byte         ||    ||                 16-byte                      || N-byte
// ------------------------------------------------------------------------------------------...
//   nanosecond timestamp    ^^                   message ID                       message body
//                        (uint16)
//                         2-byte
//                        attempts
func decodeMessage(b []byte) (*Message, error) {
	var msg Message

	if len(b) < minValidMsgLength {
		return nil, fmt.Errorf("invalid message buffer size (%d)", len(b))
	}

	msg.Timestamp = int64(binary.BigEndian.Uint64(b[:8]))
	msg.Attempts = binary.BigEndian.Uint16(b[8:10])
	copy(msg.ID[:], b[10:10+MsgIDLength])
	msg.Body = b[10+MsgIDLength:]

	return &msg, nil
}

func writeMessageToBackend(buf *bytes.Buffer, msg *Message, bq BackendQueue) error {
	buf.Reset()
	_, err := msg.WriteTo(buf)
	if err != nil {
		return err
	}
	return bq.Put(buf.Bytes())//传的时候是引用。
	/*
	   这里的put是同步的，也必须是同步的，因为传的是引用，而相应的空间会被复用，若没有确定已写入就返回的话，势必造成数据混乱。

	   见： 
	   https://github.com/nsqio/go-diskqueue/blob/master/diskqueue.go#L141
	   https://github.com/nsqio/go-diskqueue/blob/master/diskqueue.go#L628
	*/
}

/*   
  writeMessageToBackend 是将消息写入备用队列，在这个项目中对应磁盘
  该函数会被频繁调用，这里我们关心内存的使用效率问题。
  buf.Bytes()：
  func (b *Buffer) Bytes() []byte { return b.buf[b.off:] } //从go源代码中copy过来，见http://docs.studygolang.com/src/bytes/buffer.go?s=1861:1865#L47
 
  这里返回的是一个slice ,b.off的值应该为0 ，在golang里 slice为引用，非copy,下面的代码可以证明：
    buf := []byte("123456")
	fmt.Println(buf)
	buf2 := buf[2:]
	buf2[1] = 10
	fmt.Println(buf)
	fmt.Println(buf2)

  打印：
  [49 50 51 52 53 54]
  [49 50 51 10 53 54]
  [51 10 53 54]	

  可见，对buf2的修改也反应到了buf上，所以应该是引用
*/
