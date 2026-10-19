package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	configpkg "reasonix/internal/config"
)

// Reference samples belong to the user's private Reasonix home, never to the
// application bundle or a workspace. Only opaque IDs cross the synthesis API.
const voiceSampleLimit = 7_500_000 // Base64 remains below MiMo's 10 MB limit.
const voiceProfileLimit = 32

var voiceProfileID = regexp.MustCompile(`^clone:[a-f0-9]{32}$`)
var voiceProfilesMu sync.Mutex
var errVoiceSample = errors.New("声音样本无效或过大；请选择不超过 7.5 MB 的 MP3 或 WAV 录音。")
var errVoiceProfile = errors.New("声音样本无法读取；请删除该音色后重新导入录音。")

type speechVoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type storedSpeechVoice struct {
	speechVoice
	Format string `json:"format"`
	Audio  string `json:"audio"`
}
type speechVoiceCatalog struct {
	ProtocolVersion int           `json:"protocolVersion"`
	Voices          []speechVoice `json:"voices"`
}
type speechVoiceImport struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Audio  string `json:"audio"`
}

func voiceProfilesDirectory() (string, error) {
	home := configpkg.ReasonixHomeDir()
	if home == "" {
		return "", errVoiceProfile
	}
	return filepath.Join(home, "voices"), nil
}
func validVoiceName(name string) bool {
	return name == strings.TrimSpace(name) && name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 40 && !strings.ContainsFunc(name, unicode.IsControl)
}
func validateVoiceSample(format, encoded string) error {
	if len(encoded) > base64.StdEncoding.EncodedLen(voiceSampleLimit) {
		return errVoiceSample
	}
	sample, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(sample) < 44 || len(sample) > voiceSampleLimit {
		return errVoiceSample
	}
	switch format {
	case "mp3":
		if !bytes.HasPrefix(sample, []byte("ID3")) && !(sample[0] == 0xff && sample[1]&0xe0 == 0xe0) {
			return errVoiceSample
		}
	case "wav":
		if string(sample[:4]) != "RIFF" || string(sample[8:12]) != "WAVE" {
			return errVoiceSample
		}
	default:
		return errVoiceSample
	}
	return nil
}
func loadVoiceProfile(directory, id string) (storedSpeechVoice, error) {
	var voice storedSpeechVoice
	if !voiceProfileID.MatchString(id) {
		return voice, errVoiceProfile
	}
	path := filepath.Join(directory, strings.TrimPrefix(id, "clone:")+".json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 10_001_024 {
		return voice, errVoiceProfile
	}
	file, err := os.Open(path)
	if err != nil {
		return voice, errVoiceProfile
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 10_001_025))
	if decoder.Decode(&voice) != nil || voice.ID != id || !validVoiceName(voice.Name) {
		return voice, errVoiceProfile
	}
	return voice, nil
}
func listVoiceProfiles(directory string) ([]speechVoice, error) {
	voices := []speechVoice{}
	files, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return voices, nil
	}
	if err != nil {
		return nil, errVoiceProfile
	}
	for _, file := range files {
		id := "clone:" + strings.TrimSuffix(file.Name(), ".json")
		if !strings.HasSuffix(file.Name(), ".json") || !voiceProfileID.MatchString(id) {
			continue
		}
		if len(voices) >= voiceProfileLimit {
			return nil, errVoiceProfile
		}
		voice, err := loadVoiceProfile(directory, id)
		if err != nil {
			return nil, err
		}
		voices = append(voices, voice.speechVoice)
	}
	sort.Slice(voices, func(i, j int) bool {
		if voices[i].Name == voices[j].Name {
			return voices[i].ID < voices[j].ID
		}
		return voices[i].Name < voices[j].Name
	})
	return voices, nil
}
func (b *bridgeServer) speechVoices(w http.ResponseWriter, r *http.Request) {
	voiceProfilesMu.Lock()
	defer voiceProfilesMu.Unlock()
	directory, err := voiceProfilesDirectory()
	if err != nil {
		writeProtocolError(w, 500, "speech_voices", err.Error())
		return
	}
	if r.Method == http.MethodPost {
		var input speechVoiceImport
		if decodeJSONBody(w, r, 10_001_024, &input) != nil {
			writeProtocolError(w, 400, "speech_voices", errVoiceSample.Error())
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if !validVoiceName(input.Name) {
			writeProtocolError(w, 400, "speech_voices", "音色名称无效；请输入 1–40 个字符，不要包含换行。")
			return
		}
		if err := validateVoiceSample(input.Format, input.Audio); err != nil {
			writeProtocolError(w, 400, "speech_voices", err.Error())
			return
		}
		voices, err := listVoiceProfiles(directory)
		if err != nil {
			writeProtocolError(w, 500, "speech_voices", err.Error())
			return
		}
		if len(voices) >= voiceProfileLimit {
			writeProtocolError(w, 400, "speech_voices", "音色数量已达上限；请先删除不使用的音色。")
			return
		}
		for _, voice := range voices {
			if voice.Name == input.Name {
				writeProtocolError(w, 409, "speech_voices", "音色名称已存在；请换一个名称。")
				return
			}
		}
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			writeProtocolError(w, 500, "speech_voices", errVoiceProfile.Error())
			return
		}
		voice := storedSpeechVoice{speechVoice: speechVoice{ID: "clone:" + hex.EncodeToString(random[:]), Name: input.Name}, Format: input.Format, Audio: input.Audio}
		if err := saveVoiceProfile(directory, voice); err != nil {
			writeProtocolError(w, 500, "speech_voices", "音色保存失败；请检查本机存储空间后重试。")
			return
		}
		voices = append(voices, voice.speechVoice)
		writeJSON(w, 200, speechVoiceCatalog{ProtocolVersion: 1, Voices: voices})
		return
	}
	if r.Method == http.MethodDelete {
		id := r.PathValue("id")
		if !voiceProfileID.MatchString(id) {
			writeProtocolError(w, 400, "speech_voices", errVoiceProfile.Error())
			return
		}
		err := os.Remove(filepath.Join(directory, strings.TrimPrefix(id, "clone:")+".json"))
		if err != nil && !os.IsNotExist(err) {
			writeProtocolError(w, 500, "speech_voices", "音色删除失败；请检查文件权限后重试。")
			return
		}
	}
	voices, err := listVoiceProfiles(directory)
	if err != nil {
		writeProtocolError(w, 500, "speech_voices", err.Error())
		return
	}
	writeJSON(w, 200, speechVoiceCatalog{ProtocolVersion: 1, Voices: voices})
}
func saveVoiceProfile(directory string, voice storedSpeechVoice) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".voice-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(voice)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), filepath.Join(directory, strings.TrimPrefix(voice.ID, "clone:")+".json"))
}
func resolveSynthesisVoice(id string) (model, voice string, err error) {
	if speechVoices[id] {
		return "mimo-v2.5-tts", id, nil
	}
	if !voiceProfileID.MatchString(id) {
		return "", "", errVoiceProfile
	}
	voiceProfilesMu.Lock()
	defer voiceProfilesMu.Unlock()
	directory, err := voiceProfilesDirectory()
	if err != nil {
		return "", "", err
	}
	profile, err := loadVoiceProfile(directory, id)
	if err != nil {
		return "", "", err
	}
	if err := validateVoiceSample(profile.Format, profile.Audio); err != nil {
		return "", "", errVoiceProfile
	}
	mime := "audio/mpeg"
	if profile.Format == "wav" {
		mime = "audio/wav"
	}
	return "mimo-v2.5-tts-voiceclone", "data:" + mime + ";base64," + profile.Audio, nil
}
