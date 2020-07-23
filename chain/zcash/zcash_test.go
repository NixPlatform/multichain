package zcash_test

import (
	"context"
	"log"
	"os"
	"reflect"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcutil"
	"github.com/renproject/id"
	"github.com/renproject/multichain/chain/zcash"
	"github.com/renproject/multichain/compat/bitcoincompat"
	"github.com/renproject/pack"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Zcash", func() {
	Context("when submitting transactions", func() {
		Context("when sending ZEC to multiple addresses", func() {
			It("should work", func() {
				// Load private key, and assume that the associated address has
				// funds to spend. You can do this by setting ZCASH_PK to
				// the value specified in the `./multichaindeploy/.env` file.
				pkEnv := os.Getenv("ZCASH_PK")
				if pkEnv == "" {
					panic("ZCASH_PK is undefined")
				}
				wif, err := btcutil.DecodeWIF(pkEnv)
				Expect(err).ToNot(HaveOccurred())

				// PKH
				pkhAddr, err := zcash.NewAddressPubKeyHash(btcutil.Hash160(wif.PrivKey.PubKey().SerializeCompressed()), &chaincfg.RegressionNetParams)
				Expect(err).ToNot(HaveOccurred())
				pkhAddrUncompressed, err := zcash.NewAddressPubKeyHash(btcutil.Hash160(wif.PrivKey.PubKey().SerializeUncompressed()), &chaincfg.RegressionNetParams)
				Expect(err).ToNot(HaveOccurred())
				log.Printf("PKH                %v", pkhAddr.EncodeAddress())
				log.Printf("PKH (uncompressed) %v", pkhAddrUncompressed.EncodeAddress())

				// Setup the client and load the unspent transaction outputs.
				client := bitcoincompat.NewClient(bitcoincompat.DefaultClientOptions().WithHost("http://127.0.0.1:18232"))
				outputs, err := client.UnspentOutputs(context.Background(), 0, 999999999, pkhAddr)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(outputs)).To(BeNumerically(">", 0))
				output := outputs[0]

				// Check that we can load the output and that it is equal.
				// Otherwise, something strange is happening with the RPC
				// client.
				output2, _, err := client.Output(context.Background(), output.Outpoint)
				Expect(err).ToNot(HaveOccurred())
				Expect(reflect.DeepEqual(output, output2)).To(BeTrue())

				// Build the transaction by consuming the outputs and spending
				// them to a set of recipients.
				inputSigScript, err := txscript.PayToAddrScript(pkhAddr.BitcoinCompatAddress())
				Expect(err).ToNot(HaveOccurred())
				inputs := []bitcoincompat.Input{
					{
						Output:    output,
						SigScript: inputSigScript,
					},
				}
				recipients := []bitcoincompat.Recipient{
					{
						Address: pack.String(pkhAddr.EncodeAddress()),
						Value:   pack.NewU64((output.Value.Uint64() - 1000) / 2),
					},
					{
						Address: pack.String(pkhAddrUncompressed.EncodeAddress()),
						Value:   pack.NewU64((output.Value.Uint64() - 1000) / 2),
					},
				}
				tx, err := zcash.NewTxBuilder(&chaincfg.RegressionNetParams).BuildTx(inputs, recipients)
				Expect(err).ToNot(HaveOccurred())

				// Get the digests that need signing from the transaction, and
				// sign them. In production, this would be done using the RZL
				// MPC algorithm, but for the purposes of this test, using an
				// explicit privkey is ok.
				sighashes, err := tx.Sighashes()
				signatures := make([]pack.Bytes65, len(sighashes))
				Expect(err).ToNot(HaveOccurred())
				for i := range sighashes {
					hash := id.Hash(sighashes[i])
					privKey := (*id.PrivKey)(wif.PrivKey)
					signature, err := privKey.Sign(&hash)
					Expect(err).ToNot(HaveOccurred())
					signatures[i] = pack.NewBytes65(signature)
				}
				Expect(tx.Sign(signatures, pack.NewBytes(wif.SerializePubKey()))).To(Succeed())

				// Submit the transaction to the Bitcoin Cash node. Again, this
				// should be running a la `./multichaindeploy`.
				txHash, err := client.SubmitTx(context.Background(), tx)
				Expect(err).ToNot(HaveOccurred())
				log.Printf("TXID               %v", txHash)

				// Zcash nodes are a little slow, so we wait for 1000 ms before
				// beginning to check confirmations.
				time.Sleep(1 * time.Second)

				for {
					// Loop until the transaction has at least a few
					// confirmations. This implies that the transaction is
					// definitely valid, and the test has passed. We were
					// successfully able to use the multichain to construct and
					// submit a Bitcoin Cash transaction!
					confs, err := client.Confirmations(context.Background(), txHash)
					Expect(err).ToNot(HaveOccurred())
					log.Printf("                   %v/3 confirmations", confs)
					if confs >= 3 {
						break
					}
					time.Sleep(10 * time.Second)
				}

				// Check that we can load the output and that it is equal.
				// Otherwise, something strange is happening with the RPC
				// client.
				output2, _, err = client.Output(context.Background(), output.Outpoint)
				Expect(err).ToNot(HaveOccurred())
				Expect(reflect.DeepEqual(output, output2)).To(BeTrue())
			})
		})
	})
})
