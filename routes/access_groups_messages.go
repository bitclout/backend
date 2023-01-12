package routes

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/deso-protocol/core/lib"
	"github.com/pkg/errors"
)

/*type NewMessageMetadata struct {
	SenderAccessGroupOwnerPublicKey    PublicKey
	SenderAccessGroupKeyName           GroupKeyName
	SenderAccessGroupPublicKey         PublicKey
	RecipientAccessGroupOwnerPublicKey PublicKey
	RecipientAccessGroupKeyName        GroupKeyName
	RecipientAccessGroupPublicKey      PublicKey
	EncryptedText                      []byte
	TimestampNanos                     uint64
	// TODO: Add operation type create/update
	NewMessageType
	NewMessageOperation
}*/

type SendDmMessageRequest struct {
	// Public key of the direct message sender.
	// This needs to match your public key used for signing the transaction.
	SenderAccessGroupOwnerPublicKeyBase58Check string `safeForLogging:"true"`
	// AccessGroupPublicKeyBase58Check is the Public key required to participate in the access groups.
	SenderAccessGroupPublicKeyBase58Check string `safeForLogging:"true"`
	// Name of the access group to be created.
	SenderAccessGroupKeyName string `safeForLogging:"true"`

	// Public key of the direct message receiver.
	RecipientAccessGroupOwnerPublicKeyBase58Check string `safeForLogging:"true"`
	// AccessGroupPublicKeyBase58Check is the Public key required to participate in the access groups.
	RecipientAccessGroupPublicKeyBase58Check string `safeForLogging:"true"`
	// Name of the access group to be created.
	RecipientAccessGroupKeyName string `safeForLogging:"true"`

	// EncryptedMessageText is the intended message content. It is recommended to pass actual encrypted message here,
	// although unencrypted message can be passed as well.
	EncryptedMessageText []byte

	MinFeeRateNanosPerKB uint64 `safeForLogging:"true"`
	// No need to specify ProfileEntryResponse in each TransactionFee
	TransactionFees []TransactionFee `safeForLogging:"true"`
	// ExtraData is an arbitrary key value map
	ExtraData map[string]string
}

type SendDmResponse struct {
	TstampNanos uint64

	TotalInputNanos   uint64
	ChangeAmountNanos uint64
	FeeNanos          uint64
	Transaction       *lib.MsgDeSoTxn
	TransactionHex    string
}

// Base58 decodes a public key string and verifies if it is in a valid public key format.
func Base58DecodeAndValidatePublickey(publicKeyBase58Check string) (publicKeyBytes []byte, err error) {

	publicKeyBytes, _, err = lib.Base58CheckDecode(publicKeyBase58Check)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Base58DecodeAndValidatePublickey: Problem decoding "+
			"base58 public key %s: %v", publicKeyBase58Check, err))

	}

	// validate whether the access group public key is a valid public key.
	err = lib.IsByteArrayValidPublicKey(publicKeyBytes)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Base58DecodeAndValidatePublickey: Problem validating "+
			"base58 public key %s: %v", publicKeyBase58Check, err))

	}

	return publicKeyBytes, nil
}

func ValidateAccessGroupPublicKeyAndName(publicKeyBase58Check, accessGroupKeyName string) (publicKeyBytes []byte, accessGroupKeyNameBytes []byte, err error) {
	publicKeyBytes, _, err = lib.Base58CheckDecode(publicKeyBase58Check)
	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("ValidateAccessGroupPublicKeyAndName: Problem decoding "+
			"base58 public key %s: %v", publicKeyBase58Check, err))

	}
	// get the byte array of the access group key name.
	accessGroupKeyNameBytes = []byte(accessGroupKeyName)
	// Validates whether the accessGroupOwner key is a valid public key and
	// some basic checks on access group key name like Min and Max characters.
	if err = lib.ValidateAccessGroupPublicKeyAndName(accessGroupKeyNameBytes, accessGroupKeyNameBytes); err != nil {
		return nil, nil, errors.New(fmt.Sprintf("ValidateAccessGroupPublicKeyAndName: Problem validating access group owner "+
			"public key and access group key name %s: %v", accessGroupKeyName, err))

	}

	// Access group name key cannot be equal to base group name key (equal to all zeros).
	// By default all users belong to the access group with base name key.
	if lib.EqualGroupKeyName(lib.NewGroupKeyName(accessGroupKeyNameBytes), lib.BaseGroupKeyName()) {
		errors.New(fmt.Sprintf(
			"ValidateAccessGroupPublicKeyAndName: Access Group key cannot be same as base key (all zeros)."+"access group key name %s", accessGroupKeyName))
		return
	}

	return publicKeyBytes, accessGroupKeyNameBytes, nil
}

func (fes *APIServer) SendDmMessage(ww http.ResponseWriter, req *http.Request) {

	decoder := json.NewDecoder(io.LimitReader(req.Body, MaxRequestBodySizeBytes))
	requestData := SendDmMessageRequest{}
	if err := decoder.Decode(&requestData); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Problem parsing request body: %v", err))
		return
	}
	senderGroupOwnerPkBytes, senderGroupKeyNameBytes, err :=
		ValidateAccessGroupPublicKeyAndName(requestData.SenderAccessGroupOwnerPublicKeyBase58Check, requestData.SenderAccessGroupKeyName)
	// Decode the access group owner public key.
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Problem validating sender public key and access group name"+
			"base58 public key %s: %s %v",
			requestData.SenderAccessGroupOwnerPublicKeyBase58Check, requestData.SenderAccessGroupKeyName, err))
		return
	}

	recipientGroupOwnerPkBytes, recipientGroupKeyNameBytes, err :=
		ValidateAccessGroupPublicKeyAndName(requestData.RecipientAccessGroupOwnerPublicKeyBase58Check, requestData.RecipientAccessGroupKeyName)
	// Decode the access group owner public key.
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Problem validating sender public key and access group name"+
			"base58 public key %s: %s %v",
			requestData.SenderAccessGroupOwnerPublicKeyBase58Check, requestData.SenderAccessGroupKeyName, err))
		return
	}

	if bytes.Equal(senderGroupOwnerPkBytes, recipientGroupOwnerPkBytes) {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Dm sender and recipient "+
			"cannot be the same %s: %s",
			requestData.SenderAccessGroupOwnerPublicKeyBase58Check, requestData.SenderAccessGroupKeyName))
		return

	}

	senderAccessGroupPkbytes, err := Base58DecodeAndValidatePublickey(requestData.SenderAccessGroupPublicKeyBase58Check)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Problem validating sender "+
			"base58 public key %s: %v", requestData.SenderAccessGroupPublicKeyBase58Check, err))
		return
	}

	recipientAccessGroupPkbytes, err := Base58DecodeAndValidatePublickey(requestData.SenderAccessGroupPublicKeyBase58Check)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Problem validating recipient "+
			"base58 public key %s: %v", requestData.SenderAccessGroupPublicKeyBase58Check, err))
		return
	}

	// Compute the additional transaction fees as specified by the request body and the node-level fees.
	additionalOutputs, err := fes.getTransactionFee(lib.TxnTypeAccessGroup, senderGroupOwnerPkBytes, requestData.TransactionFees)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: TransactionFees specified in Request body are invalid: %v", err))
		return
	}

	extraData, err := EncodeExtraDataMap(requestData.ExtraData)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Problem encoding ExtraData: %v", err))
		return
	}

	tstamp := uint64(time.Now().UnixNano())

	// Core from the core lib to construct the transaction to create an access group.
	txn, totalInput, changeAmount, fees, err := fes.blockchain.CreateNewMessageTxn(
		senderGroupOwnerPkBytes, *lib.NewPublicKey(senderGroupOwnerPkBytes), *lib.NewGroupKeyName(senderGroupKeyNameBytes), *lib.NewPublicKey(senderAccessGroupPkbytes),
		*lib.NewPublicKey(recipientGroupOwnerPkBytes), *lib.NewGroupKeyName(recipientGroupKeyNameBytes), *lib.NewPublicKey(recipientAccessGroupPkbytes),
		requestData.EncryptedMessageText, tstamp,
		lib.NewMessageTypeDm, lib.NewMessageOperationCreate,
		extraData, requestData.MinFeeRateNanosPerKB, fes.backendServer.GetMempool(), additionalOutputs)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Problem creating transaction: %v", err))
		return
	}

	// Add node source to txn metadata
	fes.AddNodeSourceToTxnMetadata(txn)

	txnBytes, err := txn.ToBytes(true)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Problem serializing transaction: %v", err))
		return
	}

	// Return all the data associated with the transaction in the response
	res := SendDmResponse{
		TotalInputNanos:   totalInput,
		ChangeAmountNanos: changeAmount,
		FeeNanos:          fees,
		Transaction:       txn,
		TransactionHex:    hex.EncodeToString(txnBytes),
	}

	if err := json.NewEncoder(ww).Encode(res); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendDmMessage: Problem encoding response as JSON: %v", err))
		return
	}

}

func (fes *APIServer) SendGroupChatMessage(ww http.ResponseWriter, req *http.Request) {

	decoder := json.NewDecoder(io.LimitReader(req.Body, MaxRequestBodySizeBytes))
	requestData := SendDmMessageRequest{}
	if err := decoder.Decode(&requestData); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Problem parsing request body: %v", err))
		return
	}
	senderGroupOwnerPkBytes, senderGroupKeyNameBytes, err :=
		ValidateAccessGroupPublicKeyAndName(requestData.SenderAccessGroupOwnerPublicKeyBase58Check, requestData.SenderAccessGroupKeyName)
	// Decode the access group owner public key.
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Problem validating sender public key and access group name"+
			"base58 public key %s: %s %v",
			requestData.SenderAccessGroupOwnerPublicKeyBase58Check, requestData.SenderAccessGroupKeyName, err))
		return
	}

	recipientGroupOwnerPkBytes, recipientGroupKeyNameBytes, err :=
		ValidateAccessGroupPublicKeyAndName(requestData.RecipientAccessGroupOwnerPublicKeyBase58Check, requestData.RecipientAccessGroupKeyName)
	// Decode the access group owner public key.
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Problem validating sender public key and access group name"+
			"base58 public key %s: %s %v",
			requestData.SenderAccessGroupOwnerPublicKeyBase58Check, requestData.SenderAccessGroupKeyName, err))
		return
	}

	if bytes.Equal(senderGroupOwnerPkBytes, recipientGroupOwnerPkBytes) {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Dm sender and recipient "+
			"cannot be the same %s: %s",
			requestData.SenderAccessGroupOwnerPublicKeyBase58Check, requestData.SenderAccessGroupKeyName))
		return

	}

	senderAccessGroupPkbytes, err := Base58DecodeAndValidatePublickey(requestData.SenderAccessGroupPublicKeyBase58Check)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Problem validating sender "+
			"base58 public key %s: %v", requestData.SenderAccessGroupPublicKeyBase58Check, err))
		return
	}

	recipientAccessGroupPkbytes, err := Base58DecodeAndValidatePublickey(requestData.SenderAccessGroupPublicKeyBase58Check)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Problem validating recipient "+
			"base58 public key %s: %v", requestData.SenderAccessGroupPublicKeyBase58Check, err))
		return
	}

	// Compute the additional transaction fees as specified by the request body and the node-level fees.
	additionalOutputs, err := fes.getTransactionFee(lib.TxnTypeAccessGroup, senderGroupOwnerPkBytes, requestData.TransactionFees)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: TransactionFees specified in Request body are invalid: %v", err))
		return
	}

	extraData, err := EncodeExtraDataMap(requestData.ExtraData)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Problem encoding ExtraData: %v", err))
		return
	}

	tstamp := uint64(time.Now().UnixNano())

	// Core from the core lib to construct the transaction to create an access group.
	txn, totalInput, changeAmount, fees, err := fes.blockchain.CreateNewMessageTxn(
		senderGroupOwnerPkBytes, *lib.NewPublicKey(senderGroupOwnerPkBytes), *lib.NewGroupKeyName(senderGroupKeyNameBytes), *lib.NewPublicKey(senderAccessGroupPkbytes),
		*lib.NewPublicKey(recipientGroupOwnerPkBytes), *lib.NewGroupKeyName(recipientGroupKeyNameBytes), *lib.NewPublicKey(recipientAccessGroupPkbytes),
		requestData.EncryptedMessageText, tstamp,
		lib.NewMessageTypeGroupChat, lib.NewMessageOperationCreate,
		extraData, requestData.MinFeeRateNanosPerKB, fes.backendServer.GetMempool(), additionalOutputs)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Problem creating transaction: %v", err))
		return
	}

	// Add node source to txn metadata
	fes.AddNodeSourceToTxnMetadata(txn)

	txnBytes, err := txn.ToBytes(true)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Problem serializing transaction: %v", err))
		return
	}

	// Return all the data associated with the transaction in the response
	res := SendDmResponse{
		TotalInputNanos:   totalInput,
		ChangeAmountNanos: changeAmount,
		FeeNanos:          fees,
		Transaction:       txn,
		TransactionHex:    hex.EncodeToString(txnBytes),
	}

	if err := json.NewEncoder(ww).Encode(res); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("SendGroupChatMessage: Problem encoding response as JSON: %v", err))
		return
	}

}

func (fes *APIServer) fetchLatestMessageFromSingleDmThread(dmThreadKey *lib.DmThreadKey, startTimestamp uint64) (*lib.NewMessageEntry, error) {

	latestMessageEntries, err := fes.fetchMaxMessagesFromDmThread(dmThreadKey, startTimestamp, 1)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	if len(latestMessageEntries) > 0 {
		return latestMessageEntries[0], nil
	}

	return &lib.NewMessageEntry{}, nil
}

func (fes *APIServer) fetchMaxMessagesFromDmThread(dmThreadKey *lib.DmThreadKey, startTimestamp uint64, MaxMessagesToFetch int) ([]*lib.NewMessageEntry, error) {
	utxoView, err := fes.backendServer.GetMempool().GetAugmentedUniversalView()
	if err != nil {
		return nil, errors.Wrap(fmt.Errorf("getGroupOwnerAccessIdsForPublicKey: Error generating "+
			"utxo view: %v", err), "")
	}
	latestMessageEntries, err := utxoView.GetPaginatedMessageEntriesForDmThread(*dmThreadKey, startTimestamp, uint64(MaxMessagesToFetch))
	if err != nil {
		return nil, errors.Wrap(fmt.Errorf("Error fetching entries for dmThreadKey, "+
			"startTimestamp, and MaxMessagesToFetch: %v %v %v", dmThreadKey, startTimestamp, MaxMessagesToFetch), "")
	}

	return latestMessageEntries, nil
}

func (fes *APIServer) fetchLatestMessageFromDmThreads(dmThreads []*lib.DmThreadKey) ([]*lib.NewMessageEntry, error) {

	var latestMessageEntries []*lib.NewMessageEntry
	currTime := time.Now().Unix()
	for _, dmThread := range dmThreads {
		latestMessageEntry, err := fes.fetchLatestMessageFromSingleDmThread(dmThread, uint64(currTime))
		if err != nil {
			return nil, errors.Wrap(err, "")
		}

		latestMessageEntries = append(latestMessageEntries, latestMessageEntry)
	}

	return latestMessageEntries, nil
}

// Helper function retrieve access groups of the given public keys.
// Returns both the accessGroupIdsOwned , accessGroupIdsMember by the public key.
func (fes *APIServer) getAllDmThreadsForPublicKey(publicKeyBase58DecodedBytes []byte) (dmThreads []*lib.DmThreadKey, err error) {

	utxoView, err := fes.backendServer.GetMempool().GetAugmentedUniversalView()
	if err != nil {
		return nil, errors.Wrap(fmt.Errorf("getGroupOwnerAccessIdsForPublicKey: Error generating "+
			"utxo view: %v", err), "")
	}

	// call the core library and fetch group IDs owned by the public key.
	dmThreads, err = utxoView.GetAllUserDmThreads(*lib.NewPublicKey(publicKeyBase58DecodedBytes))
	if err != nil {
		return nil, errors.Wrapf(err, "getGroupOwnerAccessIdsForPublicKey: Problem getting access group ids for member")
	}

	return dmThreads, nil
}

func (fes *APIServer) fetchLatestMessageFromGroupChatThread(accessGroupId *lib.AccessGroupId, startTimestamp uint64) (*lib.NewMessageEntry, error) {

	latestMessageEntries, err := fes.fetchMaxMessagesFromGroupChatThread(accessGroupId, startTimestamp, 1)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}
	if len(latestMessageEntries) > 0 {
		return latestMessageEntries[0], nil
	}

	return &lib.NewMessageEntry{}, nil
}

func (fes *APIServer) fetchMaxMessagesFromGroupChatThread(accessGroupId *lib.AccessGroupId, startTimestamp uint64, MaxMessagesToFetch int) ([]*lib.NewMessageEntry, error) {
	utxoView, err := fes.backendServer.GetMempool().GetAugmentedUniversalView()
	if err != nil {
		return nil, errors.Wrap(fmt.Errorf("getGroupOwnerAccessIdsForPublicKey: Error generating "+
			"utxo view: %v", err), "")
	}
	latestMessageEntries, err := utxoView.GetPaginatedMessageEntriesForGroupChatThread(*accessGroupId, startTimestamp, uint64(MaxMessagesToFetch))
	if err != nil {
		return nil, errors.Wrap(fmt.Errorf("Error fetching messages for access group ID, "+
			"startTimestamp, and MaxMessagesToFetch: %v %v %v", accessGroupId, startTimestamp, MaxMessagesToFetch), "")
	}
	return latestMessageEntries, nil
}

func (fes *APIServer) fetchLatestMessageFromGroupChatThreads(groupChatThreads []*lib.AccessGroupId) ([]*lib.NewMessageEntry, error) {

	var latestMessageEntries []*lib.NewMessageEntry
	currTime := time.Now().Unix()
	for _, dmThread := range groupChatThreads {
		latestMessageEntry, err := fes.fetchLatestMessageFromGroupChatThread(dmThread, uint64(currTime))
		if err != nil {
			return nil, errors.Wrap(err, "")
		}

		latestMessageEntries = append(latestMessageEntries, latestMessageEntry)
	}
	return latestMessageEntries, nil
}

// Helper function retrieve group chat threads for a given public key.
// Returns both the accessGroupIdsOwned , accessGroupIdsMember by the public key.
func (fes *APIServer) getAllGroupChatThreadsForPublicKey(publicKeyBase58DecodedBytes []byte) (groupChatThreads []*lib.AccessGroupId, err error) {

	utxoView, err := fes.backendServer.GetMempool().GetAugmentedUniversalView()
	if err != nil {
		return nil, errors.Wrap(fmt.Errorf("getGroupOwnerAccessIdsForPublicKey: Error generating "+
			"utxo view: %v", err), "")
	}

	// call the core library and fetch group IDs owned by the public key.
	groupChatThreads, err = utxoView.GetAllUserGroupChatThreads(*lib.NewPublicKey(publicKeyBase58DecodedBytes))
	if err != nil {
		return nil, errors.Wrapf(err, "getGroupOwnerAccessIdsForPublicKey: Problem getting access group ids for member")
	}

	return groupChatThreads, nil
}

type AccessGroupInfo struct {
	OwnerPublicKeyBase58Check       string `safeForLogging:"true"`
	AccessGroupPublicKeyBase58Check string `safeForLogging:"true"`
	AccessGroupKeyName              string `safeForLogging:"true"`
}

type DmMessageInfo struct {
	EncryptedText  []byte
	TimestampNanos uint64
}

type DmThread struct {
	SenderInfo    AccessGroupInfo
	RecipientInfo AccessGroupInfo
	MessageInfo   DmMessageInfo
}

type GetUserDmThreadsRequest struct {
	// PublicKeyBase58Check is the public key whose group IDs needs to be queried.
	OwnerPublicKeyBase58Check string `safeForLogging:"true"`
}

type GetUserDmResponse struct {
	DmThreads []DmThread
}

func (fes *APIServer) GetUserDmThreadsOrderedByTimeStamp(ww http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(req.Body, MaxRequestBodySizeBytes))
	requestData := GetUserDmThreadsRequest{}
	if err := decoder.Decode(&requestData); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem parsing request body: %v", err))
		return
	}

	// Decode the access group owner public key.
	accessGroupOwnerPkBytes, _, err := lib.Base58CheckDecode(requestData.OwnerPublicKeyBase58Check)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem decoding owner"+
			"base58 public key %s: %v", requestData.OwnerPublicKeyBase58Check, err))
		return
	}

	// get all the access groups associated with the public key.
	dmThreads, err := fes.getAllDmThreadsForPublicKey(accessGroupOwnerPkBytes)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem getting access group IDs of"+
			"public key %s: %v", requestData.OwnerPublicKeyBase58Check, err))
		return
	}
	// get all the thread keys along with the latest dm message for each of them.
	latestMessagesForThreadKeys, err := fes.fetchLatestMessageFromDmThreads(dmThreads)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem getting access group IDs of"+
			"public key %s: %v", requestData.OwnerPublicKeyBase58Check, err))
		return
	}

	sort.Slice(latestMessagesForThreadKeys, func(i, j int) bool {
		return latestMessagesForThreadKeys[i].TimestampNanos > latestMessagesForThreadKeys[j].TimestampNanos
	})

	dmMessageThreads := []DmThread{}
	for _, threadMsg := range latestMessagesForThreadKeys {
		msgThread := DmThread{
			SenderInfo: AccessGroupInfo{
				OwnerPublicKeyBase58Check:       Base58EncodePublickey(threadMsg.SenderAccessGroupOwnerPublicKey.ToBytes()),
				AccessGroupPublicKeyBase58Check: Base58EncodePublickey(threadMsg.SenderAccessGroupPublicKey.ToBytes()),
				AccessGroupKeyName:              hex.EncodeToString(threadMsg.SenderAccessGroupKeyName.ToBytes()),
			},
			RecipientInfo: AccessGroupInfo{
				OwnerPublicKeyBase58Check:       Base58EncodePublickey(threadMsg.RecipientAccessGroupOwnerPublicKey.ToBytes()),
				AccessGroupPublicKeyBase58Check: Base58EncodePublickey(threadMsg.RecipientAccessGroupPublicKey.ToBytes()),
				AccessGroupKeyName:              hex.EncodeToString((threadMsg.RecipientAccessGroupKeyName.ToBytes())),
			},
			MessageInfo: DmMessageInfo{
				EncryptedText:  threadMsg.EncryptedText,
				TimestampNanos: threadMsg.TimestampNanos,
			},
		}

		dmMessageThreads = append(dmMessageThreads, msgThread)
	}

	// response containing the list of access groups.
	res := GetUserDmResponse{
		DmThreads: dmMessageThreads,
	}

	if err := json.NewEncoder(ww).Encode(res); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem encoding response as JSON: %v", err))
		return
	}
}

func Base58EncodePublickey(publickeyBytes []byte) (Base58EncodedPublickey string) {
	Base58CheckPrefix := [3]byte{0xcd, 0x14, 0x0}
	return lib.Base58CheckEncodeWithPrefix(publickeyBytes, Base58CheckPrefix)
}

type GetPaginatedMessagesForDmThreadRequest struct {
	UserGroupOwnerPublicKeyBase58Check  string
	UserGroupKeyName                    string
	PartyGroupOwnerPublicKeyBase58Check string
	PartyGroupKeyName                   string
	StartTimeStamp                      uint64
	MaxMessagesToFetch                  int
}

type GetPaginatedMessagesForDmResponse struct {
	SenderInfo    AccessGroupInfo
	RecipientInfo AccessGroupInfo
	MessageInfo   []DmMessageInfo
}

func (fes *APIServer) GetPaginatedMessagesForDmThread(ww http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(req.Body, MaxRequestBodySizeBytes))
	requestData := GetPaginatedMessagesForDmThreadRequest{}
	if err := decoder.Decode(&requestData); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForDmThread: Problem parsing request body: %v", err))
		return
	}

	if requestData.MaxMessagesToFetch < 1 {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForDmThread: MaxMessagesToFetch cannot be less than 1: %v", requestData.MaxMessagesToFetch))
		return
	}

	senderGroupOwnerPkBytes, senderGroupKeyNameBytes, err :=
		ValidateAccessGroupPublicKeyAndName(requestData.UserGroupOwnerPublicKeyBase58Check, requestData.UserGroupKeyName)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForDmThread: Problem validating "+
			"user group owner public key and access group name %s: %s %v",
			requestData.UserGroupOwnerPublicKeyBase58Check, requestData.PartyGroupOwnerPublicKeyBase58Check, err))
		return
	}

	recipientGroupOwnerPkBytes, recipientGroupKeyNameBytes, err :=
		ValidateAccessGroupPublicKeyAndName(requestData.PartyGroupOwnerPublicKeyBase58Check, requestData.PartyGroupKeyName)
	// Decode the access group owner public key.
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForDmThread: Problem validating "+
			"party group owner public key and access group name %s: %s %v",
			requestData.PartyGroupOwnerPublicKeyBase58Check, requestData.PartyGroupKeyName, err))
		return
	}

	if bytes.Equal(senderGroupOwnerPkBytes, recipientGroupOwnerPkBytes) {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForDmThread: Dm sender and recipient "+
			"cannot be the same %s: %s",
			requestData.UserGroupOwnerPublicKeyBase58Check, requestData.PartyGroupOwnerPublicKeyBase58Check))
		return

	}

	dmThreadKey := lib.MakeDmThreadKey(*lib.NewPublicKey(senderGroupKeyNameBytes), *lib.NewGroupKeyName(senderGroupKeyNameBytes),
		*lib.NewPublicKey(recipientGroupOwnerPkBytes), *lib.NewGroupKeyName(recipientGroupKeyNameBytes))

	// Fetch the max messages between the sender and the party.
	latestMessages, err := fes.fetchMaxMessagesFromDmThread(&dmThreadKey, requestData.StartTimeStamp, requestData.MaxMessagesToFetch)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForDmThread: Problem getting paginated messages for "+
			"Request Data: %s: %v", requestData, err))
		return
	}

	dms := GetPaginatedMessagesForDmResponse{
		SenderInfo: AccessGroupInfo{
			OwnerPublicKeyBase58Check: Base58EncodePublickey(senderGroupKeyNameBytes),
			AccessGroupKeyName:        hex.EncodeToString(senderGroupKeyNameBytes),
		},
		RecipientInfo: AccessGroupInfo{
			OwnerPublicKeyBase58Check: Base58EncodePublickey(recipientGroupOwnerPkBytes),
			AccessGroupKeyName:        hex.EncodeToString(recipientGroupKeyNameBytes),
		},
		MessageInfo: []DmMessageInfo{},
	}

	for _, threadMsg := range latestMessages {
		dms.MessageInfo = append(dms.MessageInfo,
			DmMessageInfo{
				EncryptedText:  threadMsg.EncryptedText,
				TimestampNanos: threadMsg.TimestampNanos,
			},
		)
	}

	// response containing dms between sender and the party.
	res := dms

	if err := json.NewEncoder(ww).Encode(res); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForDmThread: Problem encoding response as JSON: %v", err))
		return
	}

}

type GroupChatThread struct {
	SenderInfo    AccessGroupInfo
	RecipientInfo AccessGroupInfo
	MessageInfo   DmMessageInfo
}

type GetUserGroupChatResponse struct {
	GroupChatThreads []GroupChatThread
}

func (fes *APIServer) GetUserGroupChatThreadsOrderedByTimestamp(ww http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(req.Body, MaxRequestBodySizeBytes))
	requestData := GetUserDmThreadsRequest{}
	if err := decoder.Decode(&requestData); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem parsing request body: %v", err))
		return
	}

	// Decode the access group owner public key.
	accessGroupOwnerPkBytes, _, err := lib.Base58CheckDecode(requestData.OwnerPublicKeyBase58Check)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem decoding owner"+
			"base58 public key %s: %v", requestData.OwnerPublicKeyBase58Check, err))
		return
	}
	// get all the group chat threads for the public key.
	groupChatThreads, err := fes.getAllGroupChatThreadsForPublicKey(accessGroupOwnerPkBytes)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem getting access group IDs of"+
			"public key %s: %v", requestData.OwnerPublicKeyBase58Check, err))
		return
	}
	// get all the thread keys along with the latest dm message for each of them.
	latestMessagesForGroupChats, err := fes.fetchLatestMessageFromGroupChatThreads(groupChatThreads)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem getting access group IDs of"+
			"public key %s: %v", requestData.OwnerPublicKeyBase58Check, err))
		return
	}

	sort.Slice(latestMessagesForGroupChats, func(i, j int) bool {
		return latestMessagesForGroupChats[i].TimestampNanos > latestMessagesForGroupChats[j].TimestampNanos
	})

	groupChats := []GroupChatThread{}

	for _, threadMsg := range latestMessagesForGroupChats {
		groupChat := GroupChatThread{
			SenderInfo: AccessGroupInfo{
				OwnerPublicKeyBase58Check:       Base58EncodePublickey(threadMsg.SenderAccessGroupOwnerPublicKey.ToBytes()),
				AccessGroupPublicKeyBase58Check: Base58EncodePublickey(threadMsg.SenderAccessGroupPublicKey.ToBytes()),
				AccessGroupKeyName:              hex.EncodeToString(threadMsg.SenderAccessGroupKeyName.ToBytes()),
			},
			RecipientInfo: AccessGroupInfo{
				OwnerPublicKeyBase58Check:       Base58EncodePublickey(threadMsg.RecipientAccessGroupOwnerPublicKey.ToBytes()),
				AccessGroupPublicKeyBase58Check: Base58EncodePublickey(threadMsg.RecipientAccessGroupPublicKey.ToBytes()),
				AccessGroupKeyName:              hex.EncodeToString((threadMsg.RecipientAccessGroupKeyName.ToBytes())),
			},
			MessageInfo: DmMessageInfo{
				EncryptedText:  threadMsg.EncryptedText,
				TimestampNanos: threadMsg.TimestampNanos,
			},
		}

		groupChats = append(groupChats, groupChat)
	}

	// response containing the list of group chat threads with latest message
	// the group chat threads are sorted by the latest timestamp of their last message.
	res := GetUserGroupChatResponse{
		GroupChatThreads: groupChats,
	}

	if err := json.NewEncoder(ww).Encode(res); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetUserDmThreadsOrderedByTimeStamp: Problem encoding response as JSON: %v", err))
		return
	}
}

// Fetch messages from the group chat thread of a user
type GetPaginatedMessagesForGroupChatThreadRequest struct {
	AccessGroupOwnerPublicKeyBase58Check string
	AccessGroupKeyName                   string

	StartTimeStamp     uint64
	MaxMessagesToFetch int
}

type GetPaginatedMessagesForGroupChatThreadResponse struct {
	GroupChatMessages []GroupChatThread
}

func (fes *APIServer) GetPaginatedMessagesForGroupChatThread(ww http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(req.Body, MaxRequestBodySizeBytes))
	requestData := GetPaginatedMessagesForGroupChatThreadRequest{}
	if err := decoder.Decode(&requestData); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForGroupChatThread: Problem parsing request body: %v", err))
		return
	}

	if requestData.MaxMessagesToFetch < 1 {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForGroupChatThread: MaxMessagesToFetch cannot be less than 1: %v", requestData.MaxMessagesToFetch))
		return
	}

	accessGroupOwnerPkBytes, AccessGroupKeyNameBytes, err :=
		ValidateAccessGroupPublicKeyAndName(requestData.AccessGroupOwnerPublicKeyBase58Check, requestData.AccessGroupKeyName)
	// Decode the access group owner public key.
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForGroupChatThread: Problem validating "+
			"user group owner public key and access group name %s: %s %v",
			requestData.AccessGroupOwnerPublicKeyBase58Check, requestData.AccessGroupKeyName, err))
		return
	}

	accessGroupId := lib.AccessGroupId{
		AccessGroupOwnerPublicKey: *lib.NewPublicKey(accessGroupOwnerPkBytes),
		AccessGroupKeyName:        *lib.NewGroupKeyName(AccessGroupKeyNameBytes),
	}

	// Fetch the max messages between the sender and the party.
	groupChatMessages, err := fes.fetchMaxMessagesFromGroupChatThread(&accessGroupId, requestData.StartTimeStamp, requestData.MaxMessagesToFetch)
	if err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForDmThread: Problem getting paginated messages for "+
			"Request Data: %s: %v", requestData, err))
		return
	}

	messages := []GroupChatThread{}

	for _, threadMsg := range groupChatMessages {
		message := GroupChatThread{
			SenderInfo: AccessGroupInfo{
				OwnerPublicKeyBase58Check:       Base58EncodePublickey(threadMsg.SenderAccessGroupOwnerPublicKey.ToBytes()),
				AccessGroupPublicKeyBase58Check: Base58EncodePublickey(threadMsg.SenderAccessGroupPublicKey.ToBytes()),
				AccessGroupKeyName:              hex.EncodeToString(threadMsg.SenderAccessGroupKeyName.ToBytes()),
			},
			RecipientInfo: AccessGroupInfo{
				OwnerPublicKeyBase58Check:       Base58EncodePublickey(threadMsg.RecipientAccessGroupOwnerPublicKey.ToBytes()),
				AccessGroupPublicKeyBase58Check: Base58EncodePublickey(threadMsg.RecipientAccessGroupPublicKey.ToBytes()),
				AccessGroupKeyName:              hex.EncodeToString((threadMsg.RecipientAccessGroupKeyName.ToBytes())),
			},
			MessageInfo: DmMessageInfo{
				EncryptedText:  threadMsg.EncryptedText,
				TimestampNanos: threadMsg.TimestampNanos,
			},
		}

		messages = append(messages, message)
	}

	// response containing group chat messages from the given access group ID of a public key.
	res := GetPaginatedMessagesForGroupChatThreadResponse{
		GroupChatMessages: messages,
	}

	if err := json.NewEncoder(ww).Encode(res); err != nil {
		_AddBadRequestError(ww, fmt.Sprintf("GetPaginatedMessagesForDmThread: Problem encoding response as JSON: %v", err))
		return
	}
}

func (fes *APIServer) GetAllUserMessageThreads(ww http.ResponseWriter, req *http.Request) {

}
