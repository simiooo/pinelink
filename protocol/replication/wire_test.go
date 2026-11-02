package replication

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestProofCanonicalJSONPreservesLargeIntegers(t *testing.T){
	a:=json.RawMessage(`{"b":9007199254740993,"a":{"z":1,"x":"value"}}`)
	b:=json.RawMessage(`{ "a": { "x":"value", "z":1 }, "b":9007199254740993 }`)
	if !bytes.Equal(Proof("test",a),Proof("test",b)){t.Fatal("object order changed a proof")}
	if !bytes.Contains(Proof("test",a),[]byte("9007199254740993")){t.Fatal("integer precision was lost")}
}
func TestPageProofCoversConsumerCursorAndCiphertext(t *testing.T){
	public,key,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
	pin:=base64.StdEncoding.EncodeToString(public);root,_:=Root(pin)
	e:=Envelope{Version:1,EventID:"29b2546e-42c2-482e-a619-03ab05a39939",SourceRoot:root,StreamID:"95422422-ad51-4566-aa52-ff4ce8f71b83",Sequence:1,AggregateID:"fa688379-44eb-417d-9034-daa69f8a1d3b",Authority:"LOCAL_DEVICE",AuthorityRoot:root,Kind:"TRANSFER_CHANGED",PeerRoot:root,Recipients:[]string{root},SigningKey:pin,Ciphertext:[]byte("ciphertext")}
	e.Signature=Sign(key,e.Proof())
	p:=Page{Root:root,Epoch:e.StreamID,Through:1,Upper:1,Items:[]Delivery{{Sequence:1,Event:e}}};p.Signature=Sign(key,p.Proof())
	if p.Validate(pin,root)!=nil{t.Fatal("valid page rejected")}
	changed:=p;changed.Through=2;changed.Upper=2;if changed.Validate(pin,root)==nil{t.Fatal("cursor could be changed")}
	changed=p;changed.Root="7"+root[1:];if changed.Validate(pin,changed.Root)==nil{t.Fatal("consumer could be changed")}
	changed=p;changed.Items=append([]Delivery(nil),p.Items...);changed.Items[0].Event.Ciphertext=[]byte("modified");if changed.Validate(pin,root)==nil{t.Fatal("ciphertext could be changed")}
}
