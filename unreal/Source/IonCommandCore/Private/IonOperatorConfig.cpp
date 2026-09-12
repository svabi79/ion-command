#include "IonOperatorConfig.h"

#include "HAL/FileManager.h"
#include "Misc/ConfigCacheIni.h"
#include "Misc/FileHelper.h"
#include "Misc/Paths.h"

namespace
{
    FConfigFile GOperatorFile;
    TSet<FString> GOperatorSections;
    bool bOperatorLoaded = false;

    void NoteSection(const TCHAR* Section)
    {
        GOperatorSections.Add(Section);
    }

    void CollectSectionsFromDisk()
    {
        GOperatorSections.Reset();
        FString Text;
        if (!FFileHelper::LoadFileToString(Text, *IonOperatorConfig::IniPath()))
        {
            return;
        }
        TArray<FString> Lines;
        Text.ParseIntoArrayLines(Lines);
        for (const FString& Line : Lines)
        {
            const FString Trimmed = Line.TrimStartAndEnd();
            if (Trimmed.StartsWith(TEXT("[")) && Trimmed.EndsWith(TEXT("]")) && Trimmed.Len() > 2)
            {
                GOperatorSections.Add(Trimmed.Mid(1, Trimmed.Len() - 2));
            }
        }
    }

    FConfigFile& OperatorFile()
    {
        if (!bOperatorLoaded)
        {
            const FString Path = IonOperatorConfig::IniPath();
            if (IFileManager::Get().FileExists(*Path))
            {
                GOperatorFile.Read(Path);
                CollectSectionsFromDisk();
            }
            bOperatorLoaded = true;
        }
        return GOperatorFile;
    }

    void PersistOperatorFile()
    {
        const FString Path = IonOperatorConfig::IniPath();
        IFileManager::Get().MakeDirectory(*FPaths::GetPath(Path), true);
        GOperatorFile.Dirty = true;
        GOperatorFile.Write(Path);
    }

    bool OperatorHasSection(const TCHAR* Section)
    {
        OperatorFile();
        return GOperatorSections.Contains(Section);
    }
}

namespace IonOperatorConfig
{
    FString IniPath()
    {
        // Not under Saved/Config/<Platform>/: that directory is Unreal's own,
        // and it prunes files there on shutdown.
        //
        // Computed once and normalized. GConfig warns on every lookup with a
        // non-normalized path, and the station marker reads this each frame,
        // so an unnormalized path buries the log under hundreds of identical
        // warnings per run - which is how a real message gets missed.
        static const FString Path = []
        {
            const FString Resolved = FPaths::ConvertRelativePathToFull(
                FPaths::ProjectSavedDir() / TEXT("Config") / TEXT("IonOperator.ini"));
            // Returns the normalized path; it does not modify in place.
            // Discarding the result compiles cleanly and does nothing, which
            // is exactly how the warning survived a first attempt at this.
            return FConfigCacheIni::NormalizeConfigIniPath(Resolved);
        }();
        return Path;
    }

    void Reload()
    {
        GOperatorFile = FConfigFile();
        GOperatorSections.Reset();
        bOperatorLoaded = false;
        OperatorFile();
    }

    bool GetString(const TCHAR* Section, const TCHAR* Key, FString& OutValue)
    {
        if (OperatorFile().GetString(Section, Key, OutValue))
        {
            return true;
        }
        return GConfig && GConfig->GetString(Section, Key, OutValue, GGameIni);
    }

    bool GetDouble(const TCHAR* Section, const TCHAR* Key, double& OutValue)
    {
        FString Text;
        if (OperatorFile().GetString(Section, Key, Text) && !Text.IsEmpty())
        {
            OutValue = FCString::Atod(*Text);
            return true;
        }
        return GConfig && GConfig->GetDouble(Section, Key, OutValue, GGameIni);
    }

    bool GetBool(const TCHAR* Section, const TCHAR* Key, bool& OutValue)
    {
        FString Text;
        if (OperatorFile().GetString(Section, Key, Text) && !Text.IsEmpty())
        {
            OutValue = Text.ToBool();
            return true;
        }
        return GConfig && GConfig->GetBool(Section, Key, OutValue, GGameIni);
    }

    bool GetArray(const TCHAR* Section, const TCHAR* Key, TArray<FString>& OutValues)
    {
        // Section presence, not element count, decides who owns the value: an
        // operator who deleted every entry means an empty list, and falling
        // back on a count of zero would resurrect whatever the shipped
        // defaults happen to contain.
        if (OperatorHasSection(Section))
        {
            OperatorFile().GetArray(Section, Key, OutValues);
            return true;
        }
        return GConfig && GConfig->GetArray(Section, Key, OutValues, GGameIni) > 0;
    }

    void SetArray(const TCHAR* Section, const TCHAR* Key, const TArray<FString>& Values)
    {
        OperatorFile().SetArray(Section, Key, Values);
        NoteSection(Section);
        PersistOperatorFile();
    }

    void SetString(const TCHAR* Section, const TCHAR* Key, const FString& Value)
    {
        OperatorFile().SetString(Section, Key, *Value);
        NoteSection(Section);
        PersistOperatorFile();
    }

    FString NormalizeText(const FString& Value)
    {
        return Value.TrimStartAndEnd().ToUpper();
    }

    bool CanCommitText(const FString& Clean)
    {
        return !Clean.IsEmpty();
    }

    bool IsTextFieldChar(TCHAR Character)
    {
        const TCHAR Up = FChar::ToUpper(Character);
        return (Up >= TEXT('A') && Up <= TEXT('Z')) || (Up >= TEXT('0') && Up <= TEXT('9')) || Up == TEXT('/');
    }

    void TypeIntoBuffer(FString& Buffer, TCHAR Character, bool& bReplaceAll, int32 MaxLen)
    {
        if (!IsTextFieldChar(Character) || MaxLen <= 0)
        {
            return;
        }
        const TCHAR Up = FChar::ToUpper(Character);
        if (bReplaceAll)
        {
            Buffer.Reset();
            bReplaceAll = false;
        }
        if (Buffer.Len() < MaxLen)
        {
            Buffer.AppendChar(Up);
        }
    }
}
