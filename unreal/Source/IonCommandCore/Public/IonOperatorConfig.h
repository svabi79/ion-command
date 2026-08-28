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
namespace IonOperatorConfig
{
    // Absolute path of the operator ini (…/Saved/Config/IonOperator.ini).
    IONCOMMANDCORE_API FString IniPath();

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
}
