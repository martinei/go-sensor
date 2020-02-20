package autoprofile

import (
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"
)

var getPID = getLocalPID

func getLocalPID() string {
	log.warn("using the local process pid as a default")
	return strconv.Itoa(os.Getpid())
}

type samplerConfig struct {
	logPrefix          string
	reportOnly         bool
	maxProfileDuration int64
	maxSpanDuration    int64
	maxSpanCount       int32
	samplingInterval   int64
	reportInterval     int64
}

type sampler interface {
	resetSampler()
	startSampler() error
	stopSampler() error
	buildProfile(duration int64, timespan int64) (*Profile, error)
}

type samplerScheduler struct {
	profileRecorder  *recorder
	active           *flag
	started          *flag
	sampler          sampler
	config           samplerConfig
	samplerTimer     *timer
	reportTimer      *timer
	profileLock      *sync.Mutex
	profileStart     int64
	samplingDuration int64
	samplerStart     int64
	samplerTimeout   *timer
}

func newSamplerScheduler(profileRecorder *recorder, samp sampler, config samplerConfig) *samplerScheduler {
	pr := &samplerScheduler{
		profileRecorder:  profileRecorder,
		started:          &flag{},
		sampler:          samp,
		config:           config,
		samplerTimer:     nil,
		reportTimer:      nil,
		profileLock:      &sync.Mutex{},
		profileStart:     0,
		samplingDuration: 0,
		samplerStart:     0,
		samplerTimeout:   nil,
	}

	return pr
}

func (ss *samplerScheduler) start() {
	if !ss.started.SetIfUnset() {
		return
	}

	ss.profileLock.Lock()
	defer ss.profileLock.Unlock()

	ss.reset()

	if !ss.config.reportOnly {
		ss.samplerTimer = newTimer(0, time.Duration(ss.config.samplingInterval)*time.Second, func() {
			time.Sleep(time.Duration(rand.Int63n(ss.config.samplingInterval-ss.config.maxSpanDuration)) * time.Second)
			ss.startProfiling()
		})
	}

	ss.reportTimer = newTimer(0, time.Duration(ss.config.reportInterval)*time.Second, func() {
		ss.report()
	})
}

func (ss *samplerScheduler) stop() {
	if !ss.started.UnsetIfSet() {
		return
	}

	if ss.samplerTimer != nil {
		ss.samplerTimer.Stop()
	}

	if ss.reportTimer != nil {
		ss.reportTimer.Stop()
	}
}

func (ss *samplerScheduler) reset() {
	ss.sampler.resetSampler()
	ss.profileStart = time.Now().Unix()
	ss.samplingDuration = 0
}

func (ss *samplerScheduler) startProfiling() bool {
	if !ss.started.IsSet() {
		return false
	}

	ss.profileLock.Lock()
	defer ss.profileLock.Unlock()

	if ss.samplingDuration > ss.config.maxProfileDuration*1e9 {
		log.debug(ss.config.logPrefix, "max sampling duration reached.")
		return false
	}

	if !samplerActive.SetIfUnset() {
		return false
	}

	log.debug(ss.config.logPrefix, "starting")

	err := ss.sampler.startSampler()
	if err != nil {
		samplerActive.Unset()
		log.error(err)
		return false
	}
	ss.samplerStart = time.Now().UnixNano()
	ss.samplerTimeout = newTimer(time.Duration(ss.config.maxSpanDuration)*time.Second, 0, func() {
		ss.stopSampler()
		samplerActive.Unset()
	})

	return true
}

func (ss *samplerScheduler) stopSampler() {
	ss.profileLock.Lock()
	defer ss.profileLock.Unlock()

	if ss.samplerTimeout != nil {
		ss.samplerTimeout.Stop()
	}

	err := ss.sampler.stopSampler()
	if err != nil {
		log.error(err)
		return
	}
	log.debug(ss.config.logPrefix, "stopped")

	ss.samplingDuration += time.Now().UnixNano() - ss.samplerStart
}

func (ss *samplerScheduler) report() {
	if !ss.started.IsSet() {
		return
	}

	profileTimespan := time.Now().Unix() - ss.profileStart

	ss.profileLock.Lock()
	defer ss.profileLock.Unlock()

	if !ss.config.reportOnly && ss.samplingDuration == 0 {
		return
	}

	log.debug(ss.config.logPrefix, "recording profile")

	profile, err := ss.sampler.buildProfile(ss.samplingDuration, profileTimespan)
	if err != nil {
		log.error(err)
		return
	} else {
		if len(profile.roots) == 0 {
			log.debug(ss.config.logPrefix, "not recording empty profile")
		} else {
			externalPID := getPID()
			if externalPID != "" {
				profile.processID = externalPID
				log.debug("using external PID", externalPID)
			} else {
				log.info("external PID from agent is not available, using own PID")
			}

			ss.profileRecorder.record(profile.toMap())
			log.debug(ss.config.logPrefix, "recorded profile")
		}
	}

	ss.reset()
}
