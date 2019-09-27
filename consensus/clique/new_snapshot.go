// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package clique

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"math/rand"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/params"
	lru "github.com/hashicorp/golang-lru"
)

// OkaraSnapshot is the state of the authorization voting at a given point in time.
type OkaraSnapshot struct {
	config *params.CliqueConfig // Consensus engine parameters to fine tune behavior

	Number            uint64                     `json:"number"`     // Block number where the snapshot was created
	Hash              common.Hash                `json:"hash"`       // Block hash where the snapshot was created
	StakingMap        map[common.Address]uint64  `json:"stakingMap"` // Set of authorized signers at this moment
	StakersList       []miner.Staker             `json:"stakers"`
	WeightageMap      map[common.Address]float64 `json:"weightage"`
	Distribution      map[common.Address]Range   `json:"distribution"`
	TotalStakedAmount uint64                     `json:"totalStakedAmount"`
	FutureStakingMap  map[common.Address]uint64  `json:"futureStakingMap"`
}

// Range of distribution
type Range struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// NewBerithSnapshot creates a new snapshot with the specified startup parameters. This
// method does not initialize the set of recent signers, so only ever use if for
// the genesis block.
func NewBerithSnapshot(config *params.CliqueConfig, sigcache *lru.ARCCache, number uint64, hash common.Hash, stakingMap map[common.Address]uint64) *OkaraSnapshot {
	snap := getEmptySnapshot(config, number, hash)
	populateSnapshot(snap, stakingMap)
	return snap
}

func getEmptySnapshot(config *params.CliqueConfig, number uint64, hash common.Hash) *OkaraSnapshot {
	snap := &OkaraSnapshot{
		config:            config,
		Number:            number,
		Hash:              hash,
		StakingMap:        make(map[common.Address]uint64),
		StakersList:       make([]miner.Staker, 0),
		WeightageMap:      make(map[common.Address]float64),
		Distribution:      make(map[common.Address]Range),
		FutureStakingMap:  make(map[common.Address]uint64),
		TotalStakedAmount: 0,
	}
	return snap
}

func populateSnapshot(snap *OkaraSnapshot, stakingMap map[common.Address]uint64) {
	for key, value := range stakingMap {
		snap.StakingMap[key] = value
		snap.StakersList = append(snap.StakersList, miner.Staker{
			Address:      key,
			StakedAmount: value,
		})
		snap.TotalStakedAmount = snap.TotalStakedAmount + value
	}

	sort.Sort(StakersListAscending(snap.StakersList)) // Sorting stakers list
	calculateWeightageMap(snap)
	calculateDistributedWeightage(snap)
	snap.printMaps()
}

func calculateWeightageMap(snap *OkaraSnapshot) {
	for _, staker := range snap.StakersList {
		temp := (float64(staker.StakedAmount) / float64(snap.TotalStakedAmount)) * 100
		snap.WeightageMap[staker.Address] = temp
	}
}

func calculateDistributedWeightage(snap *OkaraSnapshot) {
	low := float64(0)
	for _, staker := range snap.StakersList {
		high := low + snap.WeightageMap[staker.Address]
		snap.Distribution[staker.Address] = Range{
			Low:  low,
			High: high,
		}
		low = high
	}
}

// store inserts the snapshot into the database.
func (s *OkaraSnapshot) store(db ethdb.Database) error {
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.Put(append([]byte("berith-"), s.Hash[:]...), blob)
}

// loadOkaraSnapshot loads an existing snapshot from the database.
func loadOkaraSnapshot(config *params.CliqueConfig, sigcache *lru.ARCCache, db ethdb.Database, hash common.Hash) (*OkaraSnapshot, error) {
	blob, err := db.Get(append([]byte("berith-"), hash[:]...))
	if err != nil {
		return nil, err
	}
	snap := new(OkaraSnapshot)
	if err := json.Unmarshal(blob, snap); err != nil {
		return nil, err
	}
	snap.config = config
	//snap.sigcache = sigcache

	return snap, nil
}

// signers retrieves the list of authorized signers in ascending order.
func (s *OkaraSnapshot) signers() []common.Address {
	sigs := make([]common.Address, 0, len(s.StakersList))
	for _, sig := range s.StakersList {
		sigs = append(sigs, sig.Address)
	}
	sort.Sort(signersAscending(sigs))
	return sigs
}

func getSeedFromNumber(number uint64) float64 {
	s := string(number)
	h := sha1.New()
	h.Write([]byte(s))
	hexForm := hex.EncodeToString(h.Sum(nil))
	splitted := hexForm[:len(hexForm)-(len(hexForm)-5)]

	i := new(big.Int)
	i.SetString(splitted, 16)
	return float64(i.Int64())
}

func (s *OkaraSnapshot) printMaps() {
	log.Info("PRINTING DATA")
	for _, staker := range s.StakersList {
		log.Info("DEBUGGING", "address", staker.Address, "amount", staker.StakedAmount, "weightage", s.WeightageMap[staker.Address], "distribution", s.Distribution[staker.Address])
	}
}

func (s *OkaraSnapshot) isInTurn(blockNumber uint64, staker common.Address) bool {
	min := float64(0)
	max := float64(100)
	randomSource := rand.New(rand.NewSource(int64(blockNumber)))
	randomNumber := randomSource.Float64()*(max-min) + min
	stakerRange := s.Distribution[staker]

	//log.Info("Method: isInTurn", "random#", randomNumber, "staker", staker, "found range", stakerRange)
	if randomNumber >= stakerRange.Low && randomNumber < stakerRange.High {
		return true
	}
	return false
}
func (s *OkaraSnapshot) isOneOfRandomSealers(blockNumber uint64, signer common.Address) bool {
	seed := getSeedFromNumber(blockNumber)
	//log.Info("isOneOfRandomSealers", "Random seed", seed)

	for i := float64(0); i < 100; i++ { // Number of allowed forks
		randNumber := getARandomNumber(seed+i, 0, float64(len(s.StakersList)-1))
		index := int(math.Round(randNumber))
		if s.StakersList[index].Address == signer {
			return true
		}
	}
	return false
}

// StakersListAscending asdf
type StakersListAscending []miner.Staker

func (sa StakersListAscending) Len() int { return len(sa) }
func (sa StakersListAscending) Less(i, j int) bool {
	if sa[i].StakedAmount > sa[j].StakedAmount {
		return true
	}
	return false
}
func (sa StakersListAscending) Swap(i, j int) { sa[i], sa[j] = sa[j], sa[i] }

func getARandomNumber(seed, min, max float64) float64 {
	randomSource := rand.New(rand.NewSource(int64(seed)))
	randomNumber := randomSource.Float64()*(max-min) + min
	return randomNumber
}

// copy creates a deep copy of the snapshot
func (s *OkaraSnapshot) copy() *OkaraSnapshot {
	cpy := &OkaraSnapshot{
		config:            s.config,
		Number:            s.Number,
		Hash:              s.Hash,
		StakingMap:        make(map[common.Address]uint64),
		StakersList:       make([]miner.Staker, len(s.StakersList)),
		WeightageMap:      make(map[common.Address]float64),
		Distribution:      make(map[common.Address]Range),
		FutureStakingMap:  make(map[common.Address]uint64),
		TotalStakedAmount: 0,
	}

	for index, value := range s.StakersList {
		cpy.StakingMap[value.Address] = s.StakingMap[value.Address]
		cpy.WeightageMap[value.Address] = s.WeightageMap[value.Address]
		cpy.Distribution[value.Address] = s.Distribution[value.Address]
		cpy.StakersList[index] = value
	}

	for key, value := range s.FutureStakingMap {
		cpy.FutureStakingMap[key] = value
	}

	return cpy
}

// apply creates a new authorization snapshot by applying the given headers to
// the original one.
func (s *OkaraSnapshot) apply(headers []*types.Header, chain consensus.ChainReader) (*OkaraSnapshot, error) {
	// Allow passing in no headers for cleaner code
	if len(headers) == 0 {
		return s, nil
	}
	// Sanity check that the headers can be applied
	for i := 0; i < len(headers)-1; i++ {
		if headers[i+1].Number.Uint64() != headers[i].Number.Uint64()+1 {
			return nil, errInvalidVotingChain
		}
	}
	if headers[0].Number.Uint64() != s.Number+1 {
		return nil, errInvalidVotingChain
	}
	// Iterate through the headers and create a new snapshot
	snap := s.copy()

	var (
		start  = time.Now()
		logged = time.Now()
	)

	for i, header := range headers {
		// Resolve the authorization key and check against signers
		signer := header.Coinbase
		if _, ok := snap.StakingMap[signer]; !ok {
			return nil, errUnauthorizedSigner
		}

		for key, value := range snap.FutureStakingMap {
			log.Info("=========================DEBUGGING", "address", hex.EncodeToString(key.Bytes()), "amount", value)
		}
		if header.Number.Uint64()%s.config.Epoch != 0 {
			newFutureSigners := miner.Decode(header.Extra)
			for key, value := range newFutureSigners {
				snap.FutureStakingMap[key] = value // TODO: this will replace an old entry if the signer has more than 1 staking tx.
			}

		} else {
			newStakingMap := snap.FutureStakingMap
			snap = getEmptySnapshot(snap.config, snap.Number, snap.Hash)
			populateSnapshot(snap, newStakingMap)
		}

		// If we're taking too much time (ecrecover), notify the user once a while
		if time.Since(logged) > 8*time.Second {
			log.Info("Reconstructing snapshot ", "processed", i, "total", len(headers), "elapsed", common.PrettyDuration(time.Since(start)))
			logged = time.Now()
		}
	}
	if time.Since(start) > 8*time.Second {
		log.Info("Reconstructed snapshot", "processed", len(headers), "elapsed", common.PrettyDuration(time.Since(start)))
	}
	snap.Number += uint64(len(headers))
	snap.Hash = headers[len(headers)-1].Hash()

	return snap, nil
}
