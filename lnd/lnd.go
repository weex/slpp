package lnd

import (
	"fmt"
	"io/ioutil"
	"log"

	// "os"
	// "os/user"
	"time"

	pb "github.com/weex/slpp/gateways/lnd"
	"github.com/weex/slpp/macaroons"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	macaroon "gopkg.in/macaroon.v2"
)

// this will have lnd specific config
type Config struct {
	DataDir string
}

const (
	address  = "localhost:10009"
	certFile = "tls.cert"
	macFile  = "admin.macaroon"
)

func InitLnd(cfg Config) (*LndLn, error) {
	// get user's home
	// var homeDir string

	// user, err := user.Current()
	// if err == nil {
	//    homeDir = user.HomeDir
	// } else {
	//    homeDir = os.Getenv("HOME")
	// }

	// get macaroon
	macBytes, err := ioutil.ReadFile(cfg.DataDir + "/" + macFile)
	if err != nil {
		log.Fatalf("could not find macaroon: %v", err)
	}
	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(macBytes); err != nil {
		log.Fatalf("could not unmarshall macaroon: %v", err)
	}

	// Set up a connection to lnd.
	creds, err := credentials.NewClientTLSFromFile(cfg.DataDir+"/"+certFile, "")
	if err != nil {
		log.Fatalf("could not get creds: %v", err)
	}

	credMac := macaroons.NewMacaroonCredential(mac)

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(credMac),
	}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	// defer conn.Close()
	l := &LndLn{client: pb.NewLightningClient(conn)}

	err = l.SubscribeInvoices()
	if err != nil {
		log.Fatalf("did not subscribe to invoices")
	}

	return l, nil
}

type LndLn struct {
	client pb.LightningClient
}

func (l *LndLn) ListPeers() (*pb.ListPeersResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	r, err := l.client.ListPeers(ctx, &pb.ListPeersRequest{})
	if err != nil {
		log.Fatalf("could not get peers: %v", err)
	}
	for _, peer := range r.Peers {
		log.Printf("Peer. %s", peer)
	}
	return r, nil
}

type MakeInvoiceResponse struct {
	Payment_hash    string `json:"payment_hash,omitempty"`
	Payment_request string `json:"payment_request,omitempty"`
}

func (l *LndLn) MakeInvoice(amt int64, desc string) (MakeInvoiceResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	invoice := &pb.Invoice{
		Memo:    desc,
		Value:   amt,
		Expiry:  120,
		Private: true,
	}

	i, err := l.client.AddInvoice(ctx, invoice)
	if err != nil {
		log.Fatalf("could not get invoice: %v", err)
	}

	d, err := l.client.DecodePayReq(ctx, &pb.PayReqString{PayReq: i.PaymentRequest})
	if err != nil {
		log.Fatalf("could not decode own invoice: %v", err)
	}

	return MakeInvoiceResponse{Payment_hash: d.PaymentHash, Payment_request: i.PaymentRequest}, nil
}

type ViewInvoiceResponse struct {
	CreationDate   int64  `json:"creation_date,omitempty"`
	PaymentRequest string `json:"pay_req,omitempty"`
	Expiry         int64  `json:"expiry,omitempty"`
	Settled        bool   `json:"settled"`
	ServerDate     int64  `json:"server_date,omitempty"`
}

func (l *LndLn) ViewInvoice(payment_hash string) (ViewInvoiceResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	p := &pb.PaymentHash{RHashStr: payment_hash}
	r, err := l.client.LookupInvoice(ctx, p)
	if err != nil {
		log.Fatalf("could not get invoice: %v", err)
	}
	return ViewInvoiceResponse{CreationDate: r.CreationDate,
		PaymentRequest: r.PaymentRequest,
		Expiry:         r.Expiry,
		Settled:        r.GetSettled(),
		ServerDate:     time.Now().Unix()}, nil
}

/* implement SubscribeInvoices */

type InvoiceSubscription struct {
	add_index    uint64
	settle_index uint64
}

func (l *LndLn) SubscribeInvoices() error {
	ctx := context.TODO()

	// TODO: store the below indices in db
	var sub pb.InvoiceSubscription
	// sub.add_index = uint64(0)
	// sub.settle_index = uint64(0)

	r, err := l.client.SubscribeInvoices(ctx, &sub)
	if err != nil {
		log.Fatalf("could not get invoice: %v", err)
	}
	go l.getInvoiceUpdates(r)
	return err
}

func (l *LndLn) getInvoiceUpdates(lc pb.Lightning_SubscribeInvoicesClient) {
	for {
		invoice, err := lc.Recv()
		if err != nil {
			fmt.Printf("oh shit Recv threw an error: %v\n", err)
		} else {
			l.processInvoiceUpdate(invoice)
		}
	}
}

func (l *LndLn) processInvoiceUpdate(in *pb.Invoice) {
	fmt.Printf("invoice update received: %v", in)
}
