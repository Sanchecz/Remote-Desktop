package ru.supportgenesis.remoteit.agent;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public final class PermissionFlowTest {
    @Test public void reportsEveryPermissionCombinationWithoutSkippingSteps() {
        PermissionFlow.State none = PermissionFlow.evaluate(false, false);
        assertEquals(0, none.completed);
        assertFalse(none.ready);
        assertFalse(none.canStartSharing);

        PermissionFlow.State controlOnly = PermissionFlow.evaluate(true, false);
        assertEquals(1, controlOnly.completed);
        assertFalse(controlOnly.ready);
        assertTrue(controlOnly.canStartSharing);

        PermissionFlow.State viewOnly = PermissionFlow.evaluate(false, true);
        assertEquals(1, viewOnly.completed);
        assertFalse(viewOnly.ready);
        assertFalse(viewOnly.canStartSharing);

        PermissionFlow.State ready = PermissionFlow.evaluate(true, true);
        assertEquals(2, ready.completed);
        assertEquals(2, ready.total);
        assertTrue(ready.ready);
        assertFalse(ready.canStartSharing);
    }

    @Test public void namesTheAccessibilitySectionForCommonAndroidVendors() {
        assertEquals("«Установленные приложения»", PermissionFlow.accessibilitySectionFor("Samsung"));
        assertEquals("«Скачанные приложения»", PermissionFlow.accessibilitySectionFor("Xiaomi"));
        assertEquals("«Скачанные приложения»", PermissionFlow.accessibilitySectionFor("Redmi"));
        assertEquals("«Скачанные приложения»", PermissionFlow.accessibilitySectionFor("POCO"));
        assertEquals("«Установленные службы»", PermissionFlow.accessibilitySectionFor("Huawei"));
        assertEquals("«Установленные службы»", PermissionFlow.accessibilitySectionFor("HONOR"));
        assertEquals("«Установленные приложения» или «Скачанные приложения»", PermissionFlow.accessibilitySectionFor("Google"));
        assertEquals("«Установленные приложения» или «Скачанные приложения»", PermissionFlow.accessibilitySectionFor(null));
    }
}
