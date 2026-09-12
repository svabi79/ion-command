#include "IonOperatorConfig.h"
#include "HAL/FileManager.h"
#include "Misc/AutomationTest.h"
#include "Misc/FileHelper.h"

#if WITH_DEV_AUTOMATION_TESTS

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonOperatorConfigRoundTripTest,
    "IONCOMMAND.Core.OperatorConfig.RoundTrip",
    EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonOperatorConfigRoundTripTest::RunTest(const FString& Parameters)
{
    const TCHAR* Section = TEXT("IonCommand.Test");
    const TCHAR* Key = TEXT("Token");
    const FString Value = TEXT("HB9HSJ");

    IonOperatorConfig::SetString(Section, Key, Value);

    FString ReadBack;
    TestTrue(TEXT("in-memory get sees the value just written"),
        IonOperatorConfig::GetString(Section, Key, ReadBack));
    TestEqual(TEXT("written token is returned"), ReadBack, Value);

    FString FileText;
    TestTrue(TEXT("operator ini exists on disk after SetString"),
        FFileHelper::LoadFileToString(FileText, *IonOperatorConfig::IniPath()));
    TestTrue(TEXT("disk file contains the token"), FileText.Contains(Value));

    IonOperatorConfig::Reload();
    FString AfterReload;
    TestTrue(TEXT("reload from disk still finds the token"),
        IonOperatorConfig::GetString(Section, Key, AfterReload));
    TestEqual(TEXT("reload does not fall back to packaged defaults"), AfterReload, Value);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonOperatorTextFieldTest,
    "IONCOMMAND.Core.OperatorConfig.TextField",
    EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonOperatorTextFieldTest::RunTest(const FString& Parameters)
{
    TestEqual(TEXT("typed identity is trimmed and uppercased"),
        IonOperatorConfig::NormalizeText(TEXT(" hb9hsj ")), FString(TEXT("HB9HSJ")));
    TestTrue(TEXT("non-empty identity can commit"), IonOperatorConfig::CanCommitText(TEXT("HB9HSJ")));
    TestFalse(TEXT("empty commit is ignored so the previous value stays"),
        IonOperatorConfig::CanCommitText(TEXT("")));

    FString Buffer = TEXT("N0CALL");
    bool bReplace = true;
    IonOperatorConfig::TypeIntoBuffer(Buffer, TEXT('h'), bReplace, 10);
    IonOperatorConfig::TypeIntoBuffer(Buffer, TEXT('b'), bReplace, 10);
    IonOperatorConfig::TypeIntoBuffer(Buffer, TEXT('9'), bReplace, 10);
    IonOperatorConfig::TypeIntoBuffer(Buffer, TEXT('h'), bReplace, 10);
    IonOperatorConfig::TypeIntoBuffer(Buffer, TEXT('s'), bReplace, 10);
    IonOperatorConfig::TypeIntoBuffer(Buffer, TEXT('j'), bReplace, 10);
    TestEqual(TEXT("first keystroke replaces the leftover default"), Buffer, FString(TEXT("HB9HSJ")));
    TestFalse(TEXT("later keystrokes append"), bReplace);

    IonOperatorConfig::TypeIntoBuffer(Buffer, TEXT('!'), bReplace, 10);
    TestEqual(TEXT("punctuation is ignored"), Buffer, FString(TEXT("HB9HSJ")));
    return true;
}

#endif
