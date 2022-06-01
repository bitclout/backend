package routes

import (
	"encoding/json"
	"fmt"
	"github.com/deso-protocol/core/lib"
	"github.com/golang/glog"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
)

type GetDAOCoinLimitOrdersRequest struct {
	DAOCoin1CreatorPublicKeyBase58Check string `safeForLogging:"true"`
	DAOCoin2CreatorPublicKeyBase58Check string `safeForLogging:"true"`
}

type GetDAOCoinLimitOrdersResponse struct {
	Orders []DAOCoinLimitOrderEntryResponse
}

type DAOCoinLimitOrderEntryResponse struct {
	TransactorPublicKeyBase58Check string `safeForLogging:"true"`

	BuyingDAOCoinCreatorPublicKeyBase58Check  string `safeForLogging:"true"`
	SellingDAOCoinCreatorPublicKeyBase58Check string `safeForLogging:"true"`

	Price    string `safeForLogging:"true"`
	Quantity string `safeForLogging:"true"`

	ExchangeRateCoinsToSellPerCoinToBuy float64 `safeForLogging:"true"` // Deprecated
	QuantityToFill                      float64 `safeForLogging:"true"` // Deprecated

	OperationType DAOCoinLimitOrderOperationTypeString

	OrderID string
}

const DESOCoinIdentifierString = "DESO"

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

	if requestData.DAOCoin1CreatorPublicKeyBase58Check == DESOCoinIdentifierString &&
		requestData.DAOCoin2CreatorPublicKeyBase58Check == DESOCoinIdentifierString {
		_AddBadRequestError(
			ww,
			fmt.Sprint("GetDAOCoinLimitOrders: Must provide either a "+
				"DAOCoin1CreatorPublicKeyBase58Check or DAOCoin2CreatorPublicKeyBase58Check "+
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

	if requestData.DAOCoin1CreatorPublicKeyBase58Check != DESOCoinIdentifierString {
		coin1PKID, err = fes.getPKIDFromPublicKeyBase58Check(
			utxoView,
			requestData.DAOCoin1CreatorPublicKeyBase58Check,
		)
		if err != nil {
			_AddBadRequestError(
				ww,
				fmt.Sprintf("GetDAOCoinLimitOrders: Invalid DAOCoin1CreatorPublicKeyBase58Check: %v", err),
			)
			return
		}
	}

	if requestData.DAOCoin2CreatorPublicKeyBase58Check != DESOCoinIdentifierString {
		coin2PKID, err = fes.getPKIDFromPublicKeyBase58Check(
			utxoView,
			requestData.DAOCoin2CreatorPublicKeyBase58Check,
		)
		if err != nil {
			_AddBadRequestError(
				ww,
				fmt.Sprintf("GetDAOCoinLimitOrders: Invalid DAOCoin2CreatorPublicKeyBase58Check: %v", err),
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
			requestData.DAOCoin1CreatorPublicKeyBase58Check,
			requestData.DAOCoin2CreatorPublicKeyBase58Check,
			ordersBuyingCoin1,
		),
		fes.buildDAOCoinLimitOrderResponsesFromEntriesForCoinPair(
			utxoView,
			requestData.DAOCoin2CreatorPublicKeyBase58Check,
			requestData.DAOCoin1CreatorPublicKeyBase58Check,
			ordersBuyingCoin2,
		)...,
	)

	if err = json.NewEncoder(ww).Encode(GetDAOCoinLimitOrdersResponse{Orders: responses}); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetDAOCoinLimitOrders: Problem encoding response as JSON: %v", err))
		return
	}
}

type GetTransactorDAOCoinLimitOrdersRequest struct {
	TransactorPublicKeyBase58Check string `safeForLogging:"true"`
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

	transactorPKID, err := fes.getPKIDFromPublicKeyBase58Check(
		utxoView,
		requestData.TransactorPublicKeyBase58Check,
	)
	if err != nil {
		_AddBadRequestError(
			ww,
			fmt.Sprintf("GetTransactorDAOCoinLimitOrders: Invalid TransactorPublicKeyBase58Check: %v", err),
		)
		return
	}

	orders, err := utxoView.GetAllDAOCoinLimitOrdersForThisTransactor(transactorPKID)
	if err != nil {
		_AddInternalServerError(ww, fmt.Sprintf("GetTransactorDAOCoinLimitOrders: Error getting limit orders: %v", err))
		return
	}

	responses := fes.buildDAOCoinLimitOrderResponsesForTransactor(utxoView, requestData.TransactorPublicKeyBase58Check, orders)

	if err = json.NewEncoder(ww).Encode(GetDAOCoinLimitOrdersResponse{Orders: responses}); err != nil {
		_AddInternalServerError(ww, fmt.Sprintf("GetTransactorDAOCoinLimitOrders: Problem encoding response as JSON: %v", err))
		return
	}
}

func (fes *APIServer) getPKIDFromPublicKeyBase58Check(
	utxoView *lib.UtxoView,
	publicKeyBase58Check string,
) (*lib.PKID, error) {
	publicKeyBytes, err := GetPubKeyBytesFromBase58Check(publicKeyBase58Check)
	if err != nil {
		return nil, err
	}

	pkid := utxoView.GetPKIDForPublicKey(publicKeyBytes).PKID

	return pkid, nil
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
		buyingCoinPublicKeyBase58Check := fes.getPublicKeyBase58CheckOrCoinIdentifierForPKID(utxoView, order.BuyingDAOCoinCreatorPKID)
		sellingCoinPublicKeyBase58Check := fes.getPublicKeyBase58CheckOrCoinIdentifierForPKID(utxoView, order.SellingDAOCoinCreatorPKID)

		response, err := buildDAOCoinLimitOrderResponse(
			transactorPublicKeyBase58Check,
			buyingCoinPublicKeyBase58Check,
			sellingCoinPublicKeyBase58Check,
			order,
		)
		if err != nil {
			glog.Errorf(
				"buildDAOCoinLimitOrderResponsesForTransactor: Unable to build DAO coin limit order response for limit order with OrderID: %v",
				order.OrderID,
			)
			continue
		}

		responses = append(responses, *response)
	}

	return responses
}

func (fes *APIServer) getPublicKeyBase58CheckOrCoinIdentifierForPKID(utxoView *lib.UtxoView, pkid *lib.PKID) string {
	base58Check := DESOCoinIdentifierString
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
	// It should not be possible to hit errors in this function. If we do hit them, it means an order with invalid
	// values made it through all validations during order creation, and was placed on the book. In
	// the read-only API endpoints, we just skip such bad orders and return all the valid orders we know of
	operationTypeString, err := orderOperationTypeToString(order.OperationType)
	if err != nil {
		return nil, err
	}

	price, err := CalculateStringPriceFromScaledExchangeRate(
		buyingCoinPublicKeyBase58Check,
		sellingCoinPublicKeyBase58Check,
		order.ScaledExchangeRateCoinsToSellPerCoinToBuy,
		operationTypeString,
	)
	if err != nil {
		return nil, err
	}

	quantity, err := CalculateQuantityAsString(
		buyingCoinPublicKeyBase58Check,
		sellingCoinPublicKeyBase58Check,
		operationTypeString,
		order.QuantityToFillInBaseUnits,
	)
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

		Price:    price,
		Quantity: quantity,

		ExchangeRateCoinsToSellPerCoinToBuy: exchangeRate,
		QuantityToFill:                      quantityToFill,

		OperationType: operationTypeString,

		OrderID: order.OrderID.String(),
	}, nil
}

func CalculateScaledExchangeRateFromPriceString(
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	price string,
	operationType lib.DAOCoinLimitOrderOperationType,
) (*uint256.Int, error) {
	// Price is expected to be a decimal string in the form "1.23456" that represents the exchange rate between the buying
	// and selling coin. If the operation type is ASK, then price represents the exchange rate of the buying coin per
	// selling coin. If the operation type is BID, then price represents the exchange rate of the selling coin per buying
	// coin.
	rawScaledPRice, err := lib.CalculateScaledExchangeRateFromString(price)
	if err != nil {
		return nil, err
	}
	if rawScaledPRice.IsZero() {
		return nil, errors.Errorf("The value %v is too small to produce a scaled exchange rate", price)
	}

	// Take the inverse of the exchange rate if the operation type is BID
	rawScaledExchangeRate := rawScaledPRice
	if operationType == lib.DAOCoinLimitOrderOperationTypeASK {
		// Here's we want to calculate 1/price into an exchange rate that's scaled by 1e38. We do this by calculating
		// the quotient (1e38 * 1e38) / (price * 1e38), which creates a scaled exchange rate with up to 38 decimal places
		// of precision which matches the level of precision in core
		oneE72 := big.NewInt(0).Mul(lib.OneE38.ToBig(), lib.OneE38.ToBig())
		rawScaledExchangeRateAsBigInt := big.NewInt(0).Div(oneE72, rawScaledPRice.ToBig())
		rawScaledExchangeRateWithPossibleOverflow, overflows := uint256.FromBig(rawScaledExchangeRateAsBigInt)
		if overflows {
			return nil, errors.Errorf("Overflow when converting %v to a scaled exchange rate", price)
		}
		rawScaledExchangeRate = rawScaledExchangeRateWithPossibleOverflow
	}

	if buyingCoinPublicKeyBase58Check == DESOCoinIdentifierString {
		// Buying coin is $DESO
		product := uint256.NewInt()
		overflow := product.MulOverflow(rawScaledExchangeRate, getDESOToDAOCoinBaseUnitsScalingFactor())
		if overflow {
			return nil, errors.Errorf("Overflow when converting %v to a scaled exchange rate", price)
		}
		return product, nil
	} else if sellingCoinPublicKeyBase58Check == DESOCoinIdentifierString {
		// Selling coin is $DESO
		quotient := uint256.NewInt().Div(rawScaledExchangeRate, getDESOToDAOCoinBaseUnitsScalingFactor())
		if quotient.IsZero() {
			return nil, errors.Errorf("The %v produces a scaled exchange rate that is too small", price)
		}
		return quotient, nil
	}
	return rawScaledExchangeRate, nil
}

// CalculateScaledExchangeRateFromFloat given a buying coin, selling coin, and a coin-level float exchange rate, this calculates
// the base unit to base unit exchange rate for the coin pair, while accounting for the difference in base unit scaling
// factors for $DESO (1e9) and DAO coins (1e18)
func CalculateScaledExchangeRateFromFloat(
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	exchangeRateCoinsToSellPerCoinToBuy float64,
) (*uint256.Int, error) {
	rawScaledExchangeRate, err := lib.CalculateScaledExchangeRateFromString(formatFloatAsString(exchangeRateCoinsToSellPerCoinToBuy))
	if err != nil {
		return nil, err
	}
	if rawScaledExchangeRate.IsZero() {
		return nil, errors.Errorf("The float value %f is too small to produce a scaled exchange rate", exchangeRateCoinsToSellPerCoinToBuy)
	}
	if buyingCoinPublicKeyBase58Check == DESOCoinIdentifierString {
		// Buying coin is $DESO
		product := uint256.NewInt()
		overflow := product.MulOverflow(rawScaledExchangeRate, getDESOToDAOCoinBaseUnitsScalingFactor())
		if overflow {
			return nil, errors.Errorf("Overflow when convering %f to a scaled exchange rate", exchangeRateCoinsToSellPerCoinToBuy)
		}
		return product, nil
	} else if sellingCoinPublicKeyBase58Check == DESOCoinIdentifierString {
		// Selling coin is $DESO
		quotient := uint256.NewInt().Div(rawScaledExchangeRate, getDESOToDAOCoinBaseUnitsScalingFactor())
		if quotient.IsZero() {
			return nil, errors.Errorf("The float value %f is too small to produce a scaled exchange rate", exchangeRateCoinsToSellPerCoinToBuy)
		}
		return quotient, nil
	}
	return rawScaledExchangeRate, nil
}

func CalculateStringPriceFromScaledExchangeRate(
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	scaledValue *uint256.Int,
	operationTypeString DAOCoinLimitOrderOperationTypeString,
) (string, error) {
	scaledValueAsBigInt := scaledValue.ToBig()
	if buyingCoinPublicKeyBase58Check == DESOCoinIdentifierString {
		scaledValueAsBigInt.Div(scaledValueAsBigInt, getDESOToDAOCoinBaseUnitsScalingFactor().ToBig())
	} else if sellingCoinPublicKeyBase58Check == DESOCoinIdentifierString {
		scaledValueAsBigInt.Mul(scaledValueAsBigInt, getDESOToDAOCoinBaseUnitsScalingFactor().ToBig())
	}

	return formatScaledUint256AsDecimalString(scaledValueAsBigInt, lib.OneE38.ToBig()), nil
}

// CalculateExchangeRateAsFloat given a buying coin, selling coin, and base unit to base unit exchange rate, this
// calculates the coin-level float exchange rate for the coin pair, while accounting for the difference in base unit
// scaling factors for $DESO (1e9) and DAO coins (1e18)
func CalculateExchangeRateAsFloat(
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	scaledValue *uint256.Int,
) (float64, error) {
	scaledValueAsBigInt := scaledValue.ToBig()
	if buyingCoinPublicKeyBase58Check == DESOCoinIdentifierString {
		scaledValueAsBigInt.Div(scaledValueAsBigInt, getDESOToDAOCoinBaseUnitsScalingFactor().ToBig())
	} else if sellingCoinPublicKeyBase58Check == DESOCoinIdentifierString {
		scaledValueAsBigInt.Mul(scaledValueAsBigInt, getDESOToDAOCoinBaseUnitsScalingFactor().ToBig())
	}

	oneE38AsBigInt := lib.OneE38.ToBig()

	whole := big.NewInt(0).Div(scaledValueAsBigInt, oneE38AsBigInt)
	decimal := big.NewInt(0).Mod(scaledValueAsBigInt, oneE38AsBigInt)
	decimalLeadingZeros := strings.Repeat("0", getNumDigits(oneE38AsBigInt)-getNumDigits(decimal)-1)

	str := fmt.Sprintf("%d.%s%d", whole, decimalLeadingZeros, decimal)
	parsedFloat, err := strconv.ParseFloat(str, 64)
	if err != nil {
		// This should never happen since we're formatting the float ourselves above
		return 0, err
	}
	return parsedFloat, nil
}

func CalculateQuantityAsString(
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	operationTypeString DAOCoinLimitOrderOperationTypeString,
	quantityToFillInBaseUnits *uint256.Int,
) (string, error) {
	if isCoinToFillDESO(
		buyingCoinPublicKeyBase58Check,
		sellingCoinPublicKeyBase58Check,
		operationTypeString,
	) {
		return formatScaledUint256AsDecimalString(quantityToFillInBaseUnits.ToBig(), big.NewInt(int64(lib.NanosPerUnit))), nil
	}
	return formatScaledUint256AsDecimalString(quantityToFillInBaseUnits.ToBig(), lib.BaseUnitsPerCoin.ToBig()), nil
}

// CalculateQuantityToFillAsFloat given a buying coin, selling coin, operationType and a float quantity in base units,
// this calculates the float coin quantity for side the operationType refers to
func CalculateQuantityToFillAsFloat(
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	operationTypeString DAOCoinLimitOrderOperationTypeString,
	quantityToFillInBaseUnits *uint256.Int,
) (float64, error) {
	if isCoinToFillDESO(
		buyingCoinPublicKeyBase58Check,
		sellingCoinPublicKeyBase58Check,
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
		// This should never happen since we're formatting the float ourselves above
		return 0, err
	}
	return parsedFloat, nil
}

// CalculateQuantityToFillAsBaseUnits given a buying coin, selling coin, operationType and a float coin quantity,
// this calculates the quantity in base units for the side the operationType refers to
func CalculateQuantityToFillAsBaseUnits(
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	operationTypeString DAOCoinLimitOrderOperationTypeString,
	quantityToFill string,
) (*uint256.Int, error) {
	if isCoinToFillDESO(
		buyingCoinPublicKeyBase58Check,
		sellingCoinPublicKeyBase58Check,
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
func calculateQuantityToFillAsDAOCoinBaseUnits(quantityToFill string) (*uint256.Int, error) {
	return calculateQuantityToFillToBaseUnitsWithScalingFactor(
		quantityToFill,
		lib.BaseUnitsPerCoin,
	)
}

// calculate (quantityToFill * 10^9)
func calculateQuantityToFillAsDESONanos(quantityToFill string) (*uint256.Int, error) {
	return calculateQuantityToFillToBaseUnitsWithScalingFactor(
		quantityToFill,
		uint256.NewInt().SetUint64(lib.NanosPerUnit),
	)
}

// calculate (quantityToFill * scalingFactor)
func calculateQuantityToFillToBaseUnitsWithScalingFactor(
	quantityToFill string,
	scalingFactor *uint256.Int,
) (*uint256.Int, error) {
	return lib.ScaleFloatFormatStringToUint256(quantityToFill, scalingFactor)
}

// given a buying coin, selling coin, and operation type, this determines if the QuantityToFill field
// for the coin the quantity field refers to is $DESO. If it's not $DESO, then it's assumed to be a DAO coin
func isCoinToFillDESO(
	buyingCoinPublicKeyBase58Check string,
	sellingCoinPublicKeyBase58Check string,
	operationTypeString DAOCoinLimitOrderOperationTypeString,
) bool {
	return buyingCoinPublicKeyBase58Check == DESOCoinIdentifierString && operationTypeString == DAOCoinLimitOrderOperationTypeStringBID ||
		sellingCoinPublicKeyBase58Check == DESOCoinIdentifierString && operationTypeString == DAOCoinLimitOrderOperationTypeStringASK
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

// returns (1e18 / 1e9), which represents the difference in scaling factor for DAO coin base units and $DESO nanos
func getDESOToDAOCoinBaseUnitsScalingFactor() *uint256.Int {
	return uint256.NewInt().Div(
		lib.BaseUnitsPerCoin,
		uint256.NewInt().SetUint64(lib.NanosPerUnit),
	)
}

// Given a value v that is a scaled uint256 with the provided scaling factor, this prints the decimal representation
// of v as a string
// Ex: if v = 12345 and scalingFactor = 100, then this outputs 123.45
func formatScaledUint256AsDecimalString(v *big.Int, scalingFactor *big.Int) string {
	wholeNumber := big.NewInt(0).Div(v, scalingFactor)
	decimalPart := big.NewInt(0).Mod(v, scalingFactor)

	scalingFactorDigits := getNumDigits(scalingFactor)
	decimalPartAsString := fmt.Sprintf("%d", decimalPart)
	// left pad the decimal part with zeros
	if !(decimalPart.IsUint64() && decimalPart.Uint64() == 0) && len(decimalPartAsString) != scalingFactorDigits {
		decimalLeadingZeros := strings.Repeat("0", scalingFactorDigits-len(decimalPartAsString)-1)
		decimalPartAsString = fmt.Sprintf("%v%v", decimalLeadingZeros, decimalPartAsString)
	}
	return fmt.Sprintf("%d.%v", wholeNumber, decimalPartAsString)
}

// 15 is a magic number that represents the precision supported by the IEEE-754 float64 standard.
//
// If f is large (1e15 or higher), then we truncate any values beyond the first 15 digits, as
// the lack of precision can introduce garbage when printing as string
//
// If f is small (ex: 1e-15), then we print up to 15 digits to the right of the decimal point
// to make sure we capture all digits within the supported precision, but without introducing garbage
//
// The range of supported values for f is [1e-15, 1e308] with precision for the 15 most significant digits. The
// minimum value for this range artificially set to 1e-15, but can be extended all the way 1e-308 with a bit better math
func formatFloatAsString(f float64) string {
	fAsBigInt, _ := big.NewFloat(0).SetFloat64(f).Int(nil)
	supportedPrecisionDigits := 15
	numWholeNumberDigits := getNumDigits(fAsBigInt)
	// f is small, we'll print up to 15 total digits to the right of the decimal point
	if numWholeNumberDigits <= supportedPrecisionDigits {
		return fmt.Sprintf("%."+fmt.Sprintf("%d", supportedPrecisionDigits-numWholeNumberDigits)+"f", f)
	}
	// f is a large number > 1e15, so we truncate any values after the first 15 digits
	divisorToDropDigits := big.NewInt(10)
	divisorToDropDigits.Exp(divisorToDropDigits, big.NewInt(int64(numWholeNumberDigits-supportedPrecisionDigits)), nil)
	fAsBigInt.Div(fAsBigInt, divisorToDropDigits)
	fAsBigInt.Mul(fAsBigInt, divisorToDropDigits)
	return fmt.Sprintf("%d.0", fAsBigInt)
}

func getNumDigits(val *big.Int) int {
	quotient := big.NewInt(0).Set(val)
	zero := big.NewInt(0)
	ten := big.NewInt(10)
	numDigits := 0
	for quotient.Cmp(zero) != 0 {
		numDigits += 1
		quotient.Div(quotient, ten)
	}
	return numDigits
}

func (fes *APIServer) validateTransactorSellingCoinBalance(
	transactorPublicKeyBase58Check string,
	buyingDAOCoinCreatorPublicKeyBase58Check string,
	sellingDAOCoinCreatorPublicKeyBase58Check string,
	operationType DAOCoinLimitOrderOperationTypeString,
	scaledExchangeRateCoinsToSellPerCoinToBuy *uint256.Int,
	quantityToFillInBaseUnits *uint256.Int) error {
	// Validate transactor has sufficient selling coins to place
	// this new order incorporating all of their open orders.

	// Get UTXO view.
	utxoView, err := fes.backendServer.GetMempool().GetAugmentedUniversalView()
	if err != nil {
		return errors.Errorf("Problem fetching UTXOView: %v", err)
	}

	// Get transactor PKID and public key from public key base58 check.
	transactorPKID, err := fes.getPKIDFromPublicKeyBase58Check(
		utxoView, transactorPublicKeyBase58Check)
	if err != nil {
		return errors.Errorf("Invalid TransactorPublicKeyBase58Check: %v", err)
	}
	transactorPublicKey, _, err := lib.Base58CheckDecode(transactorPublicKeyBase58Check)
	if err != nil {
		return errors.Errorf("Error decoding transactor public key: %v", err)
	}

	// If buying $DESO, the buying PKID is the ZeroPKID. Else it's the DAO coin's PKID.
	buyingCoinPKID := &lib.ZeroPKID
	if buyingDAOCoinCreatorPublicKeyBase58Check != DESOCoinIdentifierString {
		buyingCoinPKID, err = fes.getPKIDFromPublicKeyBase58Check(
			utxoView, buyingDAOCoinCreatorPublicKeyBase58Check)
		if err != nil {
			return errors.Errorf("Invalid BuyingDAOCoinCreatorPublicKeyBase58Check: %v", err)
		}
	}

	// If selling $DESO, the selling PKID is the ZeroPKID. We consider this the default
	// case and update if the transactor is actually selling a DAO coin below.
	sellingCoinPKID := &lib.ZeroPKID

	// Calculate current balance for transactor.
	transactorSellingBalanceBaseUnits := uint256.NewInt()
	if sellingDAOCoinCreatorPublicKeyBase58Check == DESOCoinIdentifierString {
		// Get $DESO balance nanos.
		desoBalanceNanos, err := utxoView.GetDeSoBalanceNanosForPublicKey(transactorPublicKey)
		if err != nil {
			return errors.Errorf("Error getting transactor DESO balance: %v", err)
		}
		transactorSellingBalanceBaseUnits = uint256.NewInt().SetUint64(desoBalanceNanos)
	} else {
		// Get selling coin PKID and public key from public key base58 check.
		sellingCoinPKID, err = fes.getPKIDFromPublicKeyBase58Check(
			utxoView, sellingDAOCoinCreatorPublicKeyBase58Check)
		if err != nil {
			return errors.Errorf("Invalid SellingDAOCoinCreatorPublicKeyBase58Check: %v", err)
		}
		sellingPublicKey, _, err := lib.Base58CheckDecode(sellingDAOCoinCreatorPublicKeyBase58Check)
		if err != nil {
			return errors.Errorf("Error decoding selling public key: %v", err)
		}

		// Get DAO coin balance base units.
		balanceEntry, _, _ := utxoView.GetBalanceEntryForHODLerPubKeyAndCreatorPubKey(transactorPublicKey, sellingPublicKey, true)
		if balanceEntry == nil {
			return errors.New("Error getting transactor DAO coin balance not found")
		}
		transactorSellingBalanceBaseUnits = &balanceEntry.BalanceNanos
	}

	// Get open orders for this transactor
	orders, err := utxoView.GetAllDAOCoinLimitOrdersForThisTransactor(transactorPKID)
	if err != nil {
		return errors.Errorf("Error getting limit orders: %v", err)
	}

	// Calculate total selling quantity for current order.
	totalSellingBaseUnits := uint256.NewInt()
	if operationType == DAOCoinLimitOrderOperationTypeStringASK {
		totalSellingBaseUnits = quantityToFillInBaseUnits
	} else if operationType == DAOCoinLimitOrderOperationTypeStringBID {
		totalSellingBaseUnits, err = lib.ComputeBaseUnitsToSellUint256(
			scaledExchangeRateCoinsToSellPerCoinToBuy, quantityToFillInBaseUnits)
		if err != nil {
			return errors.Errorf("Error calculating new order selling quantity: %v", err)
		}
	} else {
		return errors.Errorf("Invalid operation type: %s", operationType)
	}

	// Add total selling quantity for existing/open orders.
	for _, order := range orders {
		if buyingCoinPKID.Eq(order.BuyingDAOCoinCreatorPKID) &&
			sellingCoinPKID.Eq(order.SellingDAOCoinCreatorPKID) {
			// Calculate selling quantity.
			orderSellingBaseUnits, err := order.BaseUnitsToSellUint256()
			if err != nil {
				return errors.Errorf("Error calculating open order selling quantity: %v", err)
			}

			// Sum selling quantity.
			totalSellingBaseUnits, err = lib.SafeUint256().Add(totalSellingBaseUnits, orderSellingBaseUnits)
			if err != nil {
				return errors.Errorf("Error adding open order selling quantity: %v", err)
			}
		}
	}

	// Compare transactor selling balance to total selling quantity.
	if transactorSellingBalanceBaseUnits.Lt(totalSellingBaseUnits) {
		return errors.Errorf("Insufficient balance to open order")
	}

	// Happy path. No error. Transactor has sufficient balance to cover their selling quantity.
	return nil
}
