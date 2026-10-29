package queue

// Fase 9.5 - revisao adversarial: vetores do pacote queue.
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado.

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func vec(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

// Q1: corrida Cancel (administrador) x abandono. Acquire acordou pelo
// ctx.Done no mesmo instante em que o slot foi concedido (select escolhe ao
// acaso); antes do abandon pegar o lock, o administrador cancela o pedido em
// execucao. O abandon encontra stDone e retorna sem emitir o registro: o
// pedido some do historico (o Release nunca e chamado, porque o proxy nao
// recebeu ticket).
func TestAdvCancelThenAbandonLosesRecord(t *testing.T) {
	var recs []Record
	var mu sync.Mutex
	m := New(Config{GenSlots: 1, OnRecord: func(r Record) { mu.Lock(); recs = append(recs, r); mu.Unlock() }})
	ctx, cancel := context.WithCancel(context.Background())
	tk, err := m.Acquire(ctx, Request{Lane: LaneGen, ClientIP: "10.0.0.1", ClientLabel: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Cancel(tk.ID); err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = m.abandon(tk, context.Canceled, StatusCanceled) // the path Acquire takes when ctx.Done wins the select
	mu.Lock()
	n := len(recs)
	mu.Unlock()
	if n != 1 {
		vec(t, "Q1-cancel-x-abandono", "Baixa", "gateway/queue/queue.go:445", fmt.Sprintf("FALHA: %d registros (esperado 1 cancelado_admin)", n))
		t.Errorf("records %d", n)
	} else {
		vec(t, "Q1-cancel-x-abandono", "Baixa", "gateway/queue/queue.go:445", "OK: 1 registro "+recs[0].Status)
	}
	if s := m.checkInvariants(); s != "" {
		t.Fatal(s)
	}
}

// Q2: inanicao de baixa. Um pedido baixa de um IP espera enquanto outro IP
// manda normal sem parar; depois de AgeAfter ele compete como normal e, pela
// justica entre IPs, tem de ser o proximo.
func TestAdvLowPriorityNotStarved(t *testing.T) {
	m := New(Config{GenSlots: 1, AgeAfter: 150 * time.Millisecond, PerClient: 64, PerIP: 64})
	hold, err := m.Acquire(context.Background(), Request{ClientIP: "10.0.0.9", ClientLabel: "h"})
	if err != nil {
		t.Fatal(err)
	}
	low := make(chan *Ticket, 1)
	go func() {
		tk, _ := m.Acquire(context.Background(), Request{ClientIP: "10.0.0.1", ClientLabel: "baixa", PriorityFinal: Low})
		low <- tk
	}()
	stop := make(chan struct{})
	var flood sync.WaitGroup
	for i := 0; i < 8; i++ {
		flood.Add(1)
		go func() {
			defer flood.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				ctx, c := context.WithTimeout(context.Background(), 400*time.Millisecond)
				tk, err := m.Acquire(ctx, Request{ClientIP: "10.0.0.2", ClientLabel: "normal"})
				c()
				if err == nil {
					time.Sleep(5 * time.Millisecond)
					tk.Release(Result{})
				}
			}
		}()
	}
	time.Sleep(200 * time.Millisecond) // past AgeAfter
	hold.Release(Result{})
	select {
	case tk := <-low:
		vec(t, "Q2-inanicao-baixa", "Media", "gateway/queue/queue.go:591", "OK: baixa envelhecido comecou apesar do fluxo continuo de normal de outro IP")
		tk.Release(Result{})
	case <-time.After(2 * time.Second):
		vec(t, "Q2-inanicao-baixa", "Media", "gateway/queue/queue.go:591", "FALHA: baixa nao comecou em 2 s")
		t.Error("starved")
	}
	close(stop)
	flood.Wait()
}

// Q3: cliente que desiste durante o escalonamento de 300 ms nao vaza slot nem
// fila.
func TestAdvCancelDuringStagger(t *testing.T) {
	m := New(Config{GenSlots: 2, StartStagger: 300 * time.Millisecond})
	a, err := m.Acquire(context.Background(), Request{ClientIP: "10.0.0.1", ClientLabel: "a"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := m.Acquire(ctx, Request{ClientIP: "10.0.0.2", ClientLabel: "b"})
		done <- err
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	err = <-done
	time.Sleep(400 * time.Millisecond) // the stagger timer fires with nothing to start
	s := m.Snapshot()
	a.Release(Result{})
	s2 := m.Snapshot()
	if err == nil || len(s.Waiting) != 0 || s.InUse[LaneGen] != 1 || s2.InUse[LaneGen] != 0 || m.checkInvariants() != "" {
		vec(t, "Q3-desiste-no-escalonamento", "Media", "gateway/queue/queue.go:680", fmt.Sprintf("FALHA: err=%v waiting=%d inuse=%d/%d", err, len(s.Waiting), s.InUse[LaneGen], s2.InUse[LaneGen]))
		t.Error("leak")
		return
	}
	vec(t, "Q3-desiste-no-escalonamento", "Media", "gateway/queue/queue.go:680", "OK: saiu da fila; slots 1 -> 0; invariantes ok")
}

// Q4: corrida promote/demote/remove/cancel x inicio/fim, com -race. Ao final
// nao sobra slot nem pedido, e cada pedido gera exatamente 1 registro.
func TestAdvActionsRaceStartEnd(t *testing.T) {
	var nrec atomic.Int64
	m := New(Config{GenSlots: 2, EmbedSlots: 1, QueueMax: 32, PerClient: 8, PerIP: 16, StartStagger: time.Millisecond,
		OnRecord: func(Record) { nrec.Add(1) }})
	var ids sync.Map
	var wg sync.WaitGroup
	var total atomic.Int64
	stop := make(chan struct{})
	for g := 0; g < 12; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(g)))
			for i := 0; i < 150; i++ {
				ctx, c := context.WithTimeout(context.Background(), (1+time.Duration(r.Intn(30)))*time.Millisecond) // >0: a ctx already dead never enters the line (no record, by design)
				id := NewID()
				ids.Store(id, true)
				total.Add(1)
				lane := LaneGen
				if r.Intn(3) == 0 {
					lane = LaneEmbed
				}
				tk, err := m.Acquire(ctx, Request{ID: id, Lane: lane, ClientIP: fmt.Sprintf("10.0.0.%d", g%3), ClientLabel: fmt.Sprintf("l%d", r.Intn(4))})
				if err == nil {
					time.Sleep(time.Duration(r.Intn(2)) * time.Millisecond)
					tk.Release(Result{})
				}
				c()
			}
		}(g)
	}
	var aw sync.WaitGroup
	aw.Add(1)
	go func() {
		defer aw.Done()
		r := rand.New(rand.NewSource(99))
		for {
			select {
			case <-stop:
				return
			default:
			}
			ids.Range(func(k, _ any) bool {
				id := k.(string)
				switch r.Intn(4) {
				case 0:
					m.Promote(id)
				case 1:
					m.Demote(id)
				case 2:
					m.Remove(id)
				case 3:
					m.Cancel(id)
				}
				return r.Intn(8) != 0
			})
			if s := m.checkInvariants(); s != "" {
				t.Error(s)
				return
			}
		}
	}()
	wg.Wait()
	close(stop)
	aw.Wait()
	time.Sleep(20 * time.Millisecond)
	s := m.Snapshot()
	if len(s.Running) != 0 || len(s.Waiting) != 0 || nrec.Load() != total.Load() {
		vec(t, "Q4-acoes-x-inicio-fim", "Alta", "gateway/queue/queue.go:738", fmt.Sprintf("FALHA: running=%d waiting=%d registros=%d pedidos=%d", len(s.Running), len(s.Waiting), nrec.Load(), total.Load()))
		t.Error("inconsistent")
		return
	}
	vec(t, "Q4-acoes-x-inicio-fim", "Alta", "gateway/queue/queue.go:738", fmt.Sprintf("OK: %d pedidos, %d registros, 0 slots presos", total.Load(), nrec.Load()))
}
