package routes

import (
	"encoding/json"
	"fmt"
	"github.com/deso-protocol/core/lib"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
)

type GetDAOCoinLimitOrdersRequest struct {
	DAOCoin1CreatorPublicKeyBase58CheckOrUsername string `safeForLogging:"true"`
	DAOCoin2CreatorPublicKeyBase58CheckOrUsername string `safeForLogging:"true"`
}

type GetDAOCoinLimitOrdersResponse struct {
	Orders []DAOCoinLimitOrderEntryResponse
}

type DAOCoinLimitOrderEntryResponse struct {
	TransactorPublicKeyBase58Check string `safeForLogging:"true"`

	BuyingDAOCoinCreatorPublicKeyBase58Check  string `safeForLogging:"true"`
	SellingDAOCoinCreatorPublicKeyBase58Check string `safeForLogging:"true"`

	ExchangeRateCoinsToSellPerCoinToBuy float64 `safeForLogging:"true"`
	QuantityToFill                      float64 `safeForLogging:"true"`

	OperationType DAOCoinLimitOrderOperationTypeString

	OrderID string
}

func (fes *APIServer) GetDAOCoinLimitOrders(ww http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(req.Body, MaxRequestBodySizeBytes))
	requestData := GetDAOCoinLimitOrdersRequest{}
	if err := decoder.Decode(&requestData); err != nil {
		_AddBadRequestError(
			ww,
			fmt.Sprintf("GetDAOCoinLimitOrders: Problem parsing request body: %v", err),
		)
		return
	}

	if requestData.DAOCoin1CreatorPublicKeyBase58CheckOrUsername == "" &&
		requestData.DAOCoin2CreatorPublicKeyBase58CheckOrUsername == "" {
		_AddBadRequestError(
			ww,
			fmt.Sprint("GetDAOCoinLimitOrders: Must provide either a "+
				"DAOCoin1CreatorPublicKeyBase58CheckOrUsername or DAOCoin2CreatorPublicKeyBase58CheckOrUsername "+
				"or both"),
		)
		return
	}

	utxoView, err := fes.backendServer.GetMempool().GetAugmentedUniversalView()
	if err != nil {
		_AddInternalServerError(ww, fmt.Sprintf("GetDAOCoinLimitOrders: Problem fetching utxoView: %v", err))
		return
	}

	coin1PKID := &lib.ZeroPKID
	coin2PKID := &lib.ZeroPKID

	coin1ProfilePublicBase58Check := ""
	coin2ProfilePublicBase58Check := ""

	if requestData.DAOCoin1CreatorPublicKeyBase58CheckOrUsername != "" {
		coin1ProfilePublicBase58Check, coin1PKID, err = fes.getPublicKeyBase58CheckAndPKIDForPublicKeyBase58CheckOrUsername(
			utxoView,
			requestData.DAOCoin1CreatorPublicKeyBase58CheckOrUsername,
		)
		if err != nil {
			_AddBadRequestError(
				ww,
				fmt.Sprintf("GetDAOCoinLimitOrders: Invalid DAOCoin1CreatorPublicKeyBase58CheckOrUsername: %v", err),
			)
			return
		}
	}

	if requestData.DAOCoin2CreatorPublicKeyBase58CheckOrUsername != "" {
		coin2ProfilePublicBase58Check, coin2PKID, err = fes.getPublicKeyBase58CheckAndPKIDForPublicKeyBase58CheckOrUsername(
			utxoView,
			requestData.DAOCoin2CreatorPublicKeyBase58CheckOrUsername,
		)
		if err != nil {
			_AddBadRequestError(
				ww,
				fmt.Sprintf("GetDAOCoinLimitOrders: Invalid DAOCoin2CreatorPublicKeyBase58CheckOrUsername: %v", err),
			)
			return
		}
	}

	ordersBuyingCoin1, err := utxoView.GetAllDAOCoinLimitOrdersForThisDAOCoinPair(coin1PKID, coin2PKID)
	if err != nil {
		_AddInternalServerError(ww, fmt.Sprintf("GetDAOCoinLimitOrders: Error getting limit orders: %v", err))
		return
	}

	ordersBuyingCoin2, err := utxoView.GetAllDAOCoinLimitOrdersForThisDAOCoinPair(coin2PKID, coin1PKID)
	if err != nil {
		_AddInternalServerError(ww, fmt.Sprintf("GetDAOCoinLimitOrders: Error getting limit orders: %v", err))
		return
	}

	responses := append(
		fes.buildDAOCoinLimitOrderResponsesFromEntriesForCoinPair(
			utxoView,
			coin1ProfilePublicBase58Check,
			coin2ProfilePublicBase58Check,
			ordersBuyingCoin1,
		),
		fes.buildDAOCoinLimitOrderResponsesFromEntriesForCoinPair(
			utxoView,
			coin2ProfilePublicBase58Check,
			coin1ProfilePublicBase58Check,
			ordersBuyingCoin2,
		)...,
	)

	if err = json.NewEncoder(ww).Encode(GetDAOCoinLimitOrdersResponse{Orders: responses}); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetDAOCoinLimitOrders: Problem encoding response as JSON: %v", err))
		return
	}
}

type GetTransactorDAOCoinLimitOrdersRequest struct {
	TransactorPublicKeyBase58CheckOrUsername string `safeForLogging:"true"`
}

func (fes *APIServer) GetTransactorDAOCoinLimitOrders(ww http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(req.Body, MaxRequestBodySizeBytes))
	requestData := GetTransactorDAOCoinLimitOrdersRequest{}
	if err := decoder.Decode(&requestData); err != nil {
		_AddBadRequestError(
			ww,
			fmt.Sprintf("GetTransactorDAOCoinLimitOrders: Problem parsing request body: %v", err),
		)
		return
	}

	utxoView, err := fes.backendServer.GetMempool().GetAugmentedUniversalView()
	if err != nil {
		_AddInternalServerError(ww, fmt.Sprintf("GetTransactorDAOCoinLimitOrders: Problem fetching utxoView: %v", err))
		return
	}

	transactorPublicKeyBase58Check, transactorPKID, err := fes.getPublicKeyBase58CheckAndPKIDForPublicKeyBase58CheckOrUsername(
		utxoView,
		requestData.TransactorPublicKeyBase58CheckOrUsername,
	)
	if err != nil {
		_AddBadRequestError(
			ww,
			fmt.Sprintf("GetTransactorDAOCoinLimitOrders: Invalid TransactorPublicKeyBase58CheckOrUsername: %v", err),
		)
		return
	}

	orders, err := utxoView.GetAllDAOCoinLimitOrdersForThisTransactor(transactorPKID)
	if err != nil {
		_AddInternalServerError(ww, fmt.Sprintf("GetTransactorDAOCoinLimitOrders: Error getting limit orders: %v", err))
		return
	}

	responses := fes.buildDAOCoinLimitOrderResponsesForTransactor(utxoView, transactorPublicKeyBase58Check, orders)

	if err = json.NewEncoder(ww).Encode(GetDAOCoinLimitOrdersResponse{Orders: responses}); err != nil {
		_AddInternalServerError(ww, fmt.Sprintf("GetTransactorDAOCoinLimitOrders: Problem encoding response as JSON: %v", err))
		return
	}
}

func (fes *APIServer) getPublicKeyBase58CheckAndPKIDForPublicKeyBase58CheckOrUsername(
	utxoView *lib.UtxoView,
	publicKeyBase58CheckOrUsername string,
) (string, *lib.PKID, error) {
	publicKeyBytes, _, err := fes.GetPubKeyAndProfileEntryForUsernameOrPublicKeyBase58Check(
		publicKeyBase58CheckOrUsername,
		utxoView,
	)
	if err != nil {
		return "", nil, err
	}

	pkid := utxoView.GetPKIDForPublicKey(publicKeyBytes).PKID
	publicKeyBase58Check := lib.Base58CheckEncode(publicKeyBytes, false, fes.Params)

	return publicKeyBase58Check, pkid, nil
}

func (fes *APIServer) buildDAOCoinLimitOrderResponsesFromEntriesForCoinPair(
	utxoView *lib.UtxoView,
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	orders []*lib.DAOCoinLimitOrderEntry,
) []DAOCoinLimitOrderEntryResponse {
	var responses []DAOCoinLimitOrderEntryResponse

	for _, order := range orders {
		transactorPublicKey := utxoView.GetPublicKeyForPKID(order.TransactorPKID)

		response, err := buildDAOCoinLimitOrderResponse(
			lib.Base58CheckEncode(transactorPublicKey, false, fes.Params),
			buyingCoinPublicKeyBase58Check,
			sellingCoinPublicKeyBase58Check,
			order,
		)
		if err != nil {
			continue
		}

		responses = append(responses, *response)
	}

	return responses
}

func (fes *APIServer) buildDAOCoinLimitOrderResponsesForTransactor(
	utxoView *lib.UtxoView,
	transactorPublicKeyBase58Check string,
	orders []*lib.DAOCoinLimitOrderEntry,
) []DAOCoinLimitOrderEntryResponse {
	var responses []DAOCoinLimitOrderEntryResponse

	for _, order := range orders {
		buyingCoinPublicKeyBase58Check := fes.getPublicKeyBase58CheckForPKID(utxoView, order.BuyingDAOCoinCreatorPKID)
		sellingCoinPublicKeyBase58Check := fes.getPublicKeyBase58CheckForPKID(utxoView, order.SellingDAOCoinCreatorPKID)

		response, err := buildDAOCoinLimitOrderResponse(
			transactorPublicKeyBase58Check,
			buyingCoinPublicKeyBase58Check,
			sellingCoinPublicKeyBase58Check,
			order,
		)
		if err != nil {
			continue
		}

		responses = append(responses, *response)
	}

	return responses
}

func (fes *APIServer) getPublicKeyBase58CheckForPKID(utxoView *lib.UtxoView, pkid *lib.PKID) string {
	base58Check := ""
	if !pkid.IsZeroPKID() {
		base58Check = lib.Base58CheckEncode(utxoView.GetPublicKeyForPKID(pkid), false, fes.Params)
	}
	return base58Check
}

func buildDAOCoinLimitOrderResponse(
	transactorPublicKeyBase58Check string,
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	order *lib.DAOCoinLimitOrderEntry,
) (*DAOCoinLimitOrderEntryResponse, error) {
	// It should not be possible to hit errors in this function error. If we do hit them, it means an order with an
	// unsupported values made it through all validations during order creation, and was placed on the book. For
	// these read-only API endpoints, we just skip such bad orders and return all the valid orders we know of
	operationTypeString, err := orderOperationTypeToString(order.OperationType)
	if err != nil {
		return nil, err
	}

	exchangeRate, err := CalculateExchangeRateAsFloat(
		buyingCoinPublicKeyBase58Check,
		sellingCoinPublicKeyBase58Check,
		order.ScaledExchangeRateCoinsToSellPerCoinToBuy,
	)
	if err != nil {
		return nil, err
	}

	quantityToFill, err := CalculateQuantityToFillAsFloat(
		buyingCoinPublicKeyBase58Check,
		sellingCoinPublicKeyBase58Check,
		operationTypeString,
		order.QuantityToFillInBaseUnits,
	)
	if err != nil {
		return nil, err
	}

	return &DAOCoinLimitOrderEntryResponse{
		TransactorPublicKeyBase58Check: transactorPublicKeyBase58Check,

		BuyingDAOCoinCreatorPublicKeyBase58Check:  buyingCoinPublicKeyBase58Check,
		SellingDAOCoinCreatorPublicKeyBase58Check: sellingCoinPublicKeyBase58Check,
		ExchangeRateCoinsToSellPerCoinToBuy:       exchangeRate,
		QuantityToFill:                            quantityToFill,

		OperationType: operationTypeString,

		OrderID: order.OrderID.String(),
	}, nil
}

// CalculateScaledExchangeRate given a buying coin, selling coin, and a coin-level float exchange rate, this calculates
// the base unit to base unit exchange rate for the coin pair, while accounting for the difference in base unit scaling
// factors for $DESO (1e9) and DAO coins (1e18)
func CalculateScaledExchangeRate(
	buyingCoinPublicKeyBase58CheckOrUsername string,
	sellingCoinPublicKeyBase58CheckOrUsername string,
	exchangeRateCoinsToSellPerCoinToBuy float64,
) (*uint256.Int, error) {
	rawScaledExchangeRate, err := lib.CalculateScaledExchangeRateFromString(formatFloatAsString(exchangeRateCoinsToSellPerCoinToBuy))
	if err != nil {
		return nil, err
	}
	if buyingCoinPublicKeyBase58CheckOrUsername == "" {
		// Buying coin is $DESO
		product := uint256.NewInt()
		overflow := product.MulOverflow(rawScaledExchangeRate, getDESOToDAOCoinBaseUnitsScalingFactor())
		if overflow {
			return nil, errors.Errorf("Overflow when convering %f to a scaled exchange rate", exchangeRateCoinsToSellPerCoinToBuy)
		}
		return product, nil
	} else if sellingCoinPublicKeyBase58CheckOrUsername == "" {
		// Selling coin is $DESO
		quotient := uint256.NewInt().Div(rawScaledExchangeRate, getDESOToDAOCoinBaseUnitsScalingFactor())
		if quotient.Eq(uint256.NewInt().SetUint64(0)) {
			return nil, errors.Errorf("The exchange rate %f is too small to produce a scaled exchange rate", exchangeRateCoinsToSellPerCoinToBuy)
		}
		return quotient, nil
	}
	return rawScaledExchangeRate, nil
}

// CalculateExchangeRateAsFloat given a buying coin, selling coin, and base unit to base unit exchange rate, this
// calculates the coin-level float exchange rate for the coin pair, while accounting for the difference in base unit
// scaling factors for $DESO (1e9) and DAO coins (1e18)
func CalculateExchangeRateAsFloat(
	buyingCoinPublicKeyBase58CheckOrUsername string,
	sellingCoinPublicKeyBase58CheckOrUsername string,
	scaledValue *uint256.Int,
) (float64, error) {
	scaledValueCopyAsBigInt := scaledValue.ToBig()
	if buyingCoinPublicKeyBase58CheckOrUsername == "" {
		scaledValueCopyAsBigInt.Div(scaledValueCopyAsBigInt, getDESOToDAOCoinBaseUnitsScalingFactor().ToBig())
	} else if sellingCoinPublicKeyBase58CheckOrUsername == "" {
		scaledValueCopyAsBigInt.Mul(scaledValueCopyAsBigInt, getDESOToDAOCoinBaseUnitsScalingFactor().ToBig())
	}

	oneE38AsBig := lib.OneE38.ToBig()

	whole := big.NewInt(0).Div(scaledValueCopyAsBigInt, oneE38AsBig)
	decimal := big.NewInt(0).Mod(scaledValueCopyAsBigInt, oneE38AsBig)
	decimalLeadingZeros := strings.Repeat("0", getNumDigits(oneE38AsBig)-getNumDigits(decimal)-1)

	str := fmt.Sprintf("%d.%s%d", whole, decimalLeadingZeros, decimal)
	parsedFloat, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0, err
	}
	return parsedFloat, nil
}

// CalculateQuantityToFillAsFloat given a buying coin, selling coin, operationType and a float quantity in base units,
// this calculates the float coin quantity for side the operationType refers to
func CalculateQuantityToFillAsFloat(
	buyingCoinPublicKeyBase58CheckOrUsername string,
	sellingCoinPublicKeyBase58CheckOrUsername string,
	operationTypeString DAOCoinLimitOrderOperationTypeString,
	quantityToFillInBaseUnits *uint256.Int,
) (float64, error) {
	if isCoinToFillDESO(
		buyingCoinPublicKeyBase58CheckOrUsername,
		sellingCoinPublicKeyBase58CheckOrUsername,
		operationTypeString,
	) {
		return calculateQuantityToFillFromDESONanosToFloat(quantityToFillInBaseUnits)
	}
	return calculateQuantityToFillFromDAOCoinBaseUnitsToFloat(quantityToFillInBaseUnits)
}

// calculate (quantityInBaseUnits / 10^18)
func calculateQuantityToFillFromDAOCoinBaseUnitsToFloat(quantityInBaseUnits *uint256.Int) (float64, error) {
	return calculateQuantityToFillAsFloatWithScalingFactor(
		quantityInBaseUnits,
		lib.BaseUnitsPerCoin,
	)
}

// calculate (quantityInBaseUnits / 10^9)
func calculateQuantityToFillFromDESONanosToFloat(quantityInNanos *uint256.Int) (float64, error) {
	return calculateQuantityToFillAsFloatWithScalingFactor(
		quantityInNanos,
		uint256.NewInt().SetUint64(lib.NanosPerUnit),
	)
}

// calculate (quantityInBaseUnits / 10^9)
func calculateQuantityToFillAsFloatWithScalingFactor(
	quantityAsScaledValue *uint256.Int,
	scalingFactor *uint256.Int,
) (float64, error) {
	whole := uint256.NewInt().Div(quantityAsScaledValue, scalingFactor)
	decimal := uint256.NewInt().Mod(quantityAsScaledValue, scalingFactor)
	decimalLeadingZeros := strings.Repeat("0", getNumDigits(scalingFactor.ToBig())-getNumDigits(decimal.ToBig())-1)

	str := fmt.Sprintf("%d.%s%d", whole, decimalLeadingZeros, decimal)
	parsedFloat, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0, err
	}
	return parsedFloat, nil
}

// CalculateQuantityToFillAsBaseUnits given a buying coin, selling coin, operationType and a float coin quantity,
// this calculates the quantity in base units for the side the operationType refers to
func CalculateQuantityToFillAsBaseUnits(
	buyingCoinPublicKeyBase58CheckOrUsername string,
	sellingCoinPublicKeyBase58CheckOrUsername string,
	operationTypeString DAOCoinLimitOrderOperationTypeString,
	quantityToFill float64,
) (*uint256.Int, error) {
	if isCoinToFillDESO(
		buyingCoinPublicKeyBase58CheckOrUsername,
		sellingCoinPublicKeyBase58CheckOrUsername,
		operationTypeString,
	) {
		return calculateQuantityToFillAsDESONanos(
			quantityToFill,
		)
	}
	return calculateQuantityToFillAsDAOCoinBaseUnits(
		quantityToFill,
	)
}

// calculate (quantityToFill * 10^18)
func calculateQuantityToFillAsDAOCoinBaseUnits(quantityToFill float64) (*uint256.Int, error) {
	return calculateQuantityToFillToBaseUnitsWithScalingFactor(
		quantityToFill,
		lib.BaseUnitsPerCoin,
	)
}

// calculate (quantityToFill * 10^9)
func calculateQuantityToFillAsDESONanos(quantityToFill float64) (*uint256.Int, error) {
	return calculateQuantityToFillToBaseUnitsWithScalingFactor(
		quantityToFill,
		uint256.NewInt().SetUint64(lib.NanosPerUnit),
	)
}

// calculate (quantityToFill * scalingFactor)
func calculateQuantityToFillToBaseUnitsWithScalingFactor(
	quantityToFill float64,
	scalingFactor *uint256.Int,
) (*uint256.Int, error) {
	return lib.ScaleFloatFormatStringToUint256(
		formatFloatAsString(quantityToFill),
		scalingFactor,
	)
}

// isCoinToFillDESO given a buying coin, selling coin, and operation type, this determines if the QuantityToFill field
// for the coin the quantity field refers to is $DESO. If it's not $DESO, then it's assumed to be a DAO coin
func isCoinToFillDESO(
	buyingCoinPublicKeyBase58CheckOrUsername string,
	sellingCoinPublicKeyBase58CheckOrUsername string,
	operationTypeString DAOCoinLimitOrderOperationTypeString,
) bool {
	return buyingCoinPublicKeyBase58CheckOrUsername == "" && operationTypeString == DAOCoinLimitOrderOperationTypeStringBID ||
		sellingCoinPublicKeyBase58CheckOrUsername == "" && operationTypeString == DAOCoinLimitOrderOperationTypeStringASK
}

// DAOCoinLimitOrderOperationTypeString A convenience type that uses a string to represent BID / ASK side in the API,
// so it's more human-readable
type DAOCoinLimitOrderOperationTypeString string

const (
	DAOCoinLimitOrderOperationTypeStringASK DAOCoinLimitOrderOperationTypeString = "ASK"
	DAOCoinLimitOrderOperationTypeStringBID DAOCoinLimitOrderOperationTypeString = "BID"
)

func orderOperationTypeToString(
	operationType lib.DAOCoinLimitOrderOperationType,
) (DAOCoinLimitOrderOperationTypeString, error) {
	if operationType == lib.DAOCoinLimitOrderOperationTypeASK {
		return DAOCoinLimitOrderOperationTypeStringASK, nil
	}
	if operationType == lib.DAOCoinLimitOrderOperationTypeBID {
		return DAOCoinLimitOrderOperationTypeStringBID, nil
	}
	return "", errors.Errorf("Unknown DAOCoinLimitOrderOperationType %v", operationType)
}

func orderOperationTypeToUint64(
	operationType DAOCoinLimitOrderOperationTypeString,
) (lib.DAOCoinLimitOrderOperationType, error) {
	if operationType == DAOCoinLimitOrderOperationTypeStringASK {
		return lib.DAOCoinLimitOrderOperationTypeASK, nil
	}
	if operationType == DAOCoinLimitOrderOperationTypeStringBID {
		return lib.DAOCoinLimitOrderOperationTypeBID, nil
	}
	return 0, errors.Errorf("Unknown string value for DAOCoinLimitOrderOperationType %v", operationType)
}

type DAOCoinLimitOrderFillTypeString string

const (
	DAOCoinLimitOrderFillTypeGoodTillCancelled DAOCoinLimitOrderFillTypeString = "GOOD_TILL_CANCELLED"
	DAOCoinLimitOrderFillTypeFillOrKill        DAOCoinLimitOrderFillTypeString = "FILL_OR_KILL"
	DAOCoinLimitOrderFillTypeImmediateOrCancel DAOCoinLimitOrderFillTypeString = "IMMEDIATE_OR_CANCEL"
)

func orderFillTypeToUint64(
	fillType DAOCoinLimitOrderFillTypeString,
) (lib.DAOCoinLimitOrderFillType, error) {
	switch fillType {
	case DAOCoinLimitOrderFillTypeGoodTillCancelled:
		return lib.DAOCoinLimitOrderFillTypeGoodTillCancelled, nil
	case DAOCoinLimitOrderFillTypeFillOrKill:
		return lib.DAOCoinLimitOrderFillTypeFillOrKill, nil
	case DAOCoinLimitOrderFillTypeImmediateOrCancel:
		return lib.DAOCoinLimitOrderFillTypeImmediateOrCancel, nil
	}
	return 0, errors.Errorf("Unknown DAO coin limit order fill type %v", fillType)
}

func getDESOToDAOCoinBaseUnitsScalingFactor() *uint256.Int {
	return uint256.NewInt().Div(
		lib.BaseUnitsPerCoin,
		uint256.NewInt().SetUint64(lib.NanosPerUnit),
	)
}

func getNumDigits(val *big.Int) int {
	zero := big.NewInt(0)
	ten := big.NewInt(10)
	quotient := big.NewInt(0).Set(val)
	numDigits := 0
	for quotient.Cmp(zero) != 0 {
		numDigits += 1
		quotient.Div(quotient, ten)
	}
	return numDigits
}

func formatFloatAsString(f float64) string {
	numWholeNumberDigits := getNumDigits(big.NewInt(int64(f)))
	formatString := "%f"
	if numWholeNumberDigits < 15 { // 15 is a magic number for level of precision supported by float64
		formatString = "%." + fmt.Sprintf("%d", 15-numWholeNumberDigits) + "f"
	}
	return fmt.Sprintf(formatString, f)
}
