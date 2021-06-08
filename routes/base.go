package routes

import (
	"encoding/json"
	"fmt"
	"github.com/bitclout/core/lib"
	"github.com/golang/glog"
	"io"
	"net/http"
)

// Index ...
func (fes *APIServer) Index(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Your BitClout node is running!\n")
}

// NOTE: This is a readiness check not a health check
func (fes *APIServer) HealthCheck(ww http.ResponseWriter, rr *http.Request) {
	// Check that the blockchain is fully current.
	blockchainHeight := fes.blockchain.BlockTip().Height
	if fes.blockchain.ChainState() != lib.SyncStateFullyCurrent {
		_AddBadRequestError(ww, fmt.Sprintf("Waiting for blockchain to sync. "+
			"Height: %v, SyncState: %v", blockchainHeight, fes.blockchain.ChainState()))
		return
	}

	// Check that we've received our first transaction bundle.
	if !fes.backendServer.HasProcessedFirstTransactionBundle() {
		_AddBadRequestError(ww, "Waiting on mempool to sync")
		return
	}

	fmt.Fprint(ww, "200")
}

type GetExchangeRateResponse struct {
	SatoshisPerBitCloutExchangeRate        uint64
	NanosSold                              uint64
	USDCentsPerBitcoinExchangeRate         uint64
	USDCentsPerBitCloutExchangeRate        uint64
	USDCentsPerBitCloutReserveExchangeRate uint64
	BuyBitCloutFeeBasisPoints              uint64
}

func (fes *APIServer) GetExchangeRate(ww http.ResponseWriter, rr *http.Request) {
	// Get the Bitcoin to USD exchange rate by applying txns in the mempool.
	readUtxoView, _ := fes.backendServer.GetMempool().GetAugmentedUniversalView()
	usdCentsPerBitcoin := readUtxoView.GetCurrentUSDCentsPerBitcoin()

	// Get the nanos left in the tranche and the current rate of exchange.
	startNanos := readUtxoView.NanosPurchased
	satoshisPerUnit := lib.GetSatoshisPerUnitExchangeRate(
		startNanos, usdCentsPerBitcoin)

	usdCentsPerBitCloutExchangeRate, err := fes.GetExchangePrice()
	if err != nil {
		glog.Errorf("GetExchangeRate: error getting current price of BitClout from exchanges %v", err)
		usdCentsPerBitCloutExchangeRate = 0
	}
	usdCentsPerBitCloutReserveExchangeRate, err := fes.GetUSDCentsToBitCloutReserveExchangeRateFromGlobalState()
	if err != nil {
		glog.Errorf("GetExchangeRate: error getting reserve exchange rate from global state: %v", err)
		usdCentsPerBitCloutReserveExchangeRate = 0
	}

	feeBasisPoints, err := fes.GetBuyBitCloutFeeBasisPointsResponseFromGlobalState()
	if err != nil {
		glog.Errorf("GetExchangeRate: error getting buy bitclout fee basis points from global state: %v", err)
		feeBasisPoints = 0
	}

	res := &GetExchangeRateResponse{
		SatoshisPerBitCloutExchangeRate: satoshisPerUnit,
		NanosSold:                       startNanos,
		USDCentsPerBitcoinExchangeRate:  usdCentsPerBitcoin,
		USDCentsPerBitCloutExchangeRate: usdCentsPerBitCloutExchangeRate,
		USDCentsPerBitCloutReserveExchangeRate: usdCentsPerBitCloutReserveExchangeRate,
		BuyBitCloutFeeBasisPoints: feeBasisPoints,
	}

	if err = json.NewEncoder(ww).Encode(res); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetExchangeRate: Problem encoding response as JSON: %v", err))
		return
	}
}

func (fes *APIServer) GetExchangePrice() (uint64, error){
	// TODO: hit blockchain.com api to get current price
	// Return USD Cents per BitClout
	return 150 * 100, nil
}

type GetAppStateRequest struct {
	PublicKeyBase58Check string
}

type GetAppStateResponse struct {
	AmplitudeKey                        string
	AmplitudeDomain                     string
	MinSatoshisBurnedForProfileCreation uint64
	IsTestnet                           bool
	SupportEmail                        string
	ShowProcessingSpinners              bool

	HasStarterBitCloutSeed bool
	HasTwilioAPIKey        bool
	CreateProfileFeeNanos  uint64
	CompProfileCreation    bool
	DiamondLevelMap        map[int64]uint64
	HasWyreIntegration     bool

	PastDeflationBomb bool
	// Send back the password stored in our HTTPOnly cookie
	// so amplitude can track which passwords people are using
	Password string
}

func (fes *APIServer) GetAppState(ww http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(req.Body, MaxRequestBodySizeBytes))
	requestData := GetAppStateRequest{}
	if err := decoder.Decode(&requestData); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf(
			"GetAppState: Problem parsing request body: %v", err))
		return
	}

	hasTwilioAPIKey := false
	if fes.Twilio != nil {
		hasTwilioAPIKey = true
	}

	hasStarterBitCloutSeed := false
	if fes.StarterBitCloutSeed != "" {
		hasStarterBitCloutSeed = true
	}

	// Get a view with all the mempool transactions (used to get all posts / reader state).
	utxoView, err := fes.backendServer.GetMempool().GetAugmentedUniversalView()
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetAppState: Error getting augmented universal view: %v", err))
		return
	}

	res := &GetAppStateResponse{
		AmplitudeKey:                        fes.AmplitudeKey,
		AmplitudeDomain:                     fes.AmplitudeDomain,
		ShowProcessingSpinners:              fes.ShowProcessingSpinners,
		MinSatoshisBurnedForProfileCreation: fes.MinSatoshisBurnedForProfileCreation,
		IsTestnet:                           fes.Params.NetworkType == lib.NetworkType_TESTNET,
		SupportEmail:                        fes.SupportEmail,
		HasTwilioAPIKey:                     hasTwilioAPIKey,
		HasStarterBitCloutSeed:              hasStarterBitCloutSeed,
		CreateProfileFeeNanos:               utxoView.GlobalParamsEntry.CreateProfileFeeNanos,
		CompProfileCreation:                 fes.IsCompProfileCreation,
		DiamondLevelMap:                     lib.GetBitCloutNanosDiamondLevelMapAtBlockHeight(int64(fes.blockchain.BlockTip().Height)),
		HasWyreIntegration:                  fes.IsConfiguredForWyre(),
		PastDeflationBomb:                   uint64(fes.blockchain.BlockTip().Height) > fes.Params.DeflationBombBlockHeight,
	}

	if err := json.NewEncoder(ww).Encode(res); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetNotifications: Problem encoding response as JSON: %v", err))
		return
	}
}
