package realtime

import "testing"

func TestBroadcasterFansOutToAllSubscribers(t *testing.T) {
	b := NewBroadcaster(4)
	a, cancelA := b.Subscribe()
	c, cancelC := b.Subscribe()
	defer cancelA()
	defer cancelC()

	if b.SubscriberCount() != 2 {
		t.Fatalf("subscriber count = %d, want 2", b.SubscriberCount())
	}

	b.Publish(Event{Symbol: "BTCUSDT"})

	if got := (<-a).Symbol; got != "BTCUSDT" {
		t.Fatalf("subscriber a got %q", got)
	}
	if got := (<-c).Symbol; got != "BTCUSDT" {
		t.Fatalf("subscriber c got %q", got)
	}
}

func TestBroadcasterUnsubscribeClosesChannelAndIsIdempotent(t *testing.T) {
	b := NewBroadcaster(4)
	ch, cancel := b.Subscribe()

	cancel()
	cancel() // must not panic

	if _, open := <-ch; open {
		t.Fatal("channel should be closed after unsubscribe")
	}
	if b.SubscriberCount() != 0 {
		t.Fatalf("subscriber count = %d, want 0", b.SubscriberCount())
	}
}

func TestBroadcasterDropsForSlowSubscriber(t *testing.T) {
	b := NewBroadcaster(1)
	_, cancel := b.Subscribe()
	defer cancel()

	// Buffer holds 1; publishing more must not block.
	for i := 0; i < 100; i++ {
		b.Publish(Event{Symbol: "BTCUSDT"})
	}
}
