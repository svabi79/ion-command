#include "GeoCoveragePinSubsystem.h"
#include "Engine/GameInstance.h"
#include "Misc/AutomationTest.h"

#if WITH_DEV_AUTOMATION_TESTS

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonCoveragePinToggleAndCapTest, "IONCOMMAND.Data.CoveragePin.ToggleAndCap", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonCoveragePinToggleAndCapTest::RunTest(const FString& Parameters)
{
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoCoveragePinSubsystem* Pins = NewObject<UGeoCoveragePinSubsystem>(GameInstance);
    TestFalse(TEXT("empty set is unpinned"), Pins->IsPinned(TEXT("25544")));
    TestTrue(TEXT("first toggle pins"), Pins->TogglePin(TEXT("25544"), TEXT("ISS")));
    TestTrue(TEXT("pinned after toggle"), Pins->IsPinned(TEXT("25544")));
    TestEqual(TEXT("one pin stored"), Pins->GetPins().Num(), 1);
    TestFalse(TEXT("second toggle unpins"), Pins->TogglePin(TEXT("25544"), TEXT("ISS")));
    TestEqual(TEXT("empty after unpin"), Pins->GetPins().Num(), 0);

    for (int32 Index = 0; Index < UGeoCoveragePinSubsystem::MaxPins + 2; ++Index)
    {
        Pins->TogglePin(FString::Printf(TEXT("sat-%d"), Index), FString::Printf(TEXT("SAT %d"), Index));
    }
    TestEqual(TEXT("pin set stays bounded"), Pins->GetPins().Num(), UGeoCoveragePinSubsystem::MaxPins);
    TestFalse(TEXT("oldest pin was evicted"), Pins->IsPinned(TEXT("sat-0")));
    TestTrue(TEXT("newest pin is kept"), Pins->IsPinned(FString::Printf(TEXT("sat-%d"), UGeoCoveragePinSubsystem::MaxPins + 1)));
    return true;
}

#endif
