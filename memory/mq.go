// Copyright 2021 ecodeclub
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"context"
	"github.com/ecodeclub/ekit/syncx"
	"github.com/ecodeclub/mq-api"
	"sync"
)

type Topic struct {
	Name        string
	lock        sync.RWMutex
	consumerChs []chan *mq.Message
	produceChan chan *mq.Message
}

type topicOption func(topic *Topic)

func WithProducerChannelSize(size int) topicOption {
	return func(topic *Topic) {
		topic.produceChan = make(chan *mq.Message, size)
	}
}

func (t *Topic) NewConsumer(size int) mq.Consumer {
	consumerCh := make(chan *mq.Message, size)
	t.lock.Lock()
	defer t.lock.Unlock()
	t.consumerChs = append(t.consumerChs, consumerCh)
	return &TopicConsumer{
		consumerCh,
	}
}

func (t *Topic) NewProducer(topic string) mq.Producer {
	return &TopicProducer{
		ProducerCh: t.produceChan,
		topic:      topic,
	}
}

type TopicProducer struct {
	ProducerCh chan *mq.Message
	topic      string
}

func (t *TopicProducer) Produce(ctx context.Context, m *mq.Message) (*mq.ProducerResult, error) {
	m.Topic = t.topic
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case t.ProducerCh <- m:
		return &mq.ProducerResult{}, nil
	}
}

type TopicConsumer struct {
	ConsumerCh chan *mq.Message
}

func (t *TopicConsumer) Consume(ctx context.Context) (*mq.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.ConsumerCh:
		return msg, nil
	}
}

func (t *TopicConsumer) ConsumeMsgCh(ctx context.Context) (<-chan *mq.Message, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return t.ConsumerCh, nil
}

type Mq struct {
	topics syncx.Map[string, *Topic]
}

func NewMq() mq.MQ {
	return &Mq{
		syncx.Map[string, *Topic]{},
	}
}

func (m *Mq) Consumer(topic string) mq.Consumer {
	tp, _ := m.topics.LoadOrStore(topic, NewTopic(topic))
	return tp.NewConsumer(10)
}

func (m *Mq) Producer(topic string) mq.Producer {
	tp, _ := m.topics.LoadOrStore(topic, NewTopic(topic))
	return tp.NewProducer(topic)
}

func NewTopic(name string, opts ...topicOption) *Topic {
	t := &Topic{
		Name:        name,
		produceChan: make(chan *mq.Message, 1000),
	}
	for _, opt := range opts {
		opt(t)
	}
	go func() {
		for msg := range t.produceChan {
			t.lock.RLock()
			consumers := t.consumerChs
			t.lock.RUnlock()
			for _, ch := range consumers {
				ch <- msg
			}
		}
	}()
	return t
}
