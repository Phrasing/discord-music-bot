package main

import "sync"

type Song struct {
	URL       string
	ChannelID string
}

type Queue struct {
	songs []*Song
	mut   sync.Mutex
}

func NewQueue() *Queue {
	return &Queue{
		songs: make([]*Song, 0),
	}
}

func (q *Queue) Add(song *Song) {
	q.mut.Lock()
	defer q.mut.Unlock()
	q.songs = append(q.songs, song)
}

func (q *Queue) Get() *Song {
	q.mut.Lock()
	defer q.mut.Unlock()
	if len(q.songs) == 0 {
		return nil
	}
	song := q.songs[0]
	q.songs = q.songs[1:]
	return song
}

func (q *Queue) IsEmpty() bool {
	q.mut.Lock()
	defer q.mut.Unlock()
	return len(q.songs) == 0
}
