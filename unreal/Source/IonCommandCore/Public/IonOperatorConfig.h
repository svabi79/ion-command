#pragma once

#include "CoreMinimal.h"

// Operator-owned settings (station identity, personal preferences) live in
// their own ini next to the game's saved data, NOT in the engine's Game
// hierarchy.
//
// Why: Unreal rewrites the saved Game ini on shutdown and keeps only what it
// considers owned by a registered config class. A section written by plain
// GConfig calls - or provisioned by hand before first launch - is dropped, so
// the setting silently reverts to its packaged default on the next start. An
// own-station marker that reverts does not disappear: it moves to the
// placeholder grid square, asserting a position the operator never set.
//
// The file is plain ini and safe to write by hand before first launch.
//
// Reads go through an owned FConfigFile that is loaded from disk on first
// use. GConfig->GetString(customPath) does not load a file that is not
// already in its cache, so a previous session's IonOperator.ini was ignored
// and the packaged Game defaults came back.
namespace IonOperatorConfig
{
    // Absolute path of the operator ini (…/Saved/Config/IonOperator.ini).
    IONCOMMANDCORE_API FString IniPath();

    // Drop in-memory state and read the operator ini from disk again.
    // First Get/Set already loads; this is for tests and an external rewrite.
    IONCOMMANDCORE_API void Reload();

    // Reads Section/Key from the operator ini, falling back to the packaged
    // Game hierarchy so existing installs and DefaultGame.ini keep working.
    // Returns false when neither source has the key.
    IONCOMMANDCORE_API bool GetString(const TCHAR* Section, const TCHAR* Key, FString& OutValue);

    // Writes Section/Key and flushes immediately, so the value survives both
    // a crash and Unreal's shutdown rewrite of its own config files.
    IONCOMMANDCORE_API void SetString(const TCHAR* Section, const TCHAR* Key, const FString& Value);

    // Same source order (operator ini first, packaged Game hierarchy second)
    // for the remaining value shapes the settings panel persists.
    IONCOMMANDCORE_API bool GetDouble(const TCHAR* Section, const TCHAR* Key, double& OutValue);
    IONCOMMANDCORE_API bool GetBool(const TCHAR* Section, const TCHAR* Key, bool& OutValue);
    IONCOMMANDCORE_API bool GetArray(const TCHAR* Section, const TCHAR* Key, TArray<FString>& OutValues);
    IONCOMMANDCORE_API void SetArray(const TCHAR* Section, const TCHAR* Key, const TArray<FString>& Values);

    // Settings-panel text field. First typed character replaces the previous
    // value so confirming a typed identity cannot save the leftover default.
    IONCOMMANDCORE_API FString NormalizeText(const FString& Value);
    IONCOMMANDCORE_API bool CanCommitText(const FString& Clean);
    IONCOMMANDCORE_API bool IsTextFieldChar(TCHAR Character);
    IONCOMMANDCORE_API void TypeIntoBuffer(FString& Buffer, TCHAR Character, bool& bReplaceAll, int32 MaxLen);
}
