#include "IonOperatorConfig.h"

#include "Misc/ConfigCacheIni.h"
#include "Misc/Paths.h"

namespace IonOperatorConfig
{
    FString IniPath()
    {
        // Not under Saved/Config/<Platform>/: that directory is Unreal's own,
        // and it prunes files there on shutdown.
        return FPaths::ConvertRelativePathToFull(FPaths::ProjectSavedDir() / TEXT("Config") / TEXT("IonOperator.ini"));
    }

    bool GetString(const TCHAR* Section, const TCHAR* Key, FString& OutValue)
    {
        if (GConfig && GConfig->GetString(Section, Key, OutValue, IniPath()))
        {
            return true;
        }
        return GConfig && GConfig->GetString(Section, Key, OutValue, GGameIni);
    }

    bool GetDouble(const TCHAR* Section, const TCHAR* Key, double& OutValue)
    {
        if (GConfig && GConfig->GetDouble(Section, Key, OutValue, IniPath()))
        {
            return true;
        }
        return GConfig && GConfig->GetDouble(Section, Key, OutValue, GGameIni);
    }

    bool GetBool(const TCHAR* Section, const TCHAR* Key, bool& OutValue)
    {
        if (GConfig && GConfig->GetBool(Section, Key, OutValue, IniPath()))
        {
            return true;
        }
        return GConfig && GConfig->GetBool(Section, Key, OutValue, GGameIni);
    }

    bool GetArray(const TCHAR* Section, const TCHAR* Key, TArray<FString>& OutValues)
    {
        if (!GConfig)
        {
            return false;
        }
        // Section presence, not element count, decides who owns the value: an
        // operator who deleted every entry means an empty list, and falling
        // back on a count of zero would resurrect whatever the shipped
        // defaults happen to contain.
        const FString Path = IniPath();
        if (GConfig->DoesSectionExist(Section, Path))
        {
            GConfig->GetArray(Section, Key, OutValues, Path);
            return true;
        }
        return GConfig->GetArray(Section, Key, OutValues, GGameIni) > 0;
    }

    void SetArray(const TCHAR* Section, const TCHAR* Key, const TArray<FString>& Values)
    {
        if (!GConfig)
        {
            return;
        }
        const FString Path = IniPath();
        GConfig->SetArray(Section, Key, Values, Path);
        GConfig->Flush(false, Path);
    }

    void SetString(const TCHAR* Section, const TCHAR* Key, const FString& Value)
    {
        if (!GConfig)
        {
            return;
        }
        const FString Path = IniPath();
        GConfig->SetString(Section, Key, *Value, Path);
        GConfig->Flush(false, Path);
    }
}
